package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Backups: the maps folder, with the library, versions of pictures and
// all, zipped into a file. Once a folder for them is chosen, a backup is
// made each day the app runs, and the last backupsKept are kept.

const (
	backupPrefix = "MyMind 备份 "
	backupsKept  = 14
)

// zipFolder writes the files of dir, at any depth, into a zip file at dst.
// Folders named in skip are left out.
func zipFolder(dir, dst string, skip ...string) error {
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // what cannot be read is left out
		}
		if d.IsDir() {
			if path != dir && slices.Contains(skip, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || strings.HasSuffix(path, ".tmp") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(filepath.Join(filepath.Base(dir), rel))
		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// backupTo zips the maps folder into a file, in the background, and says
// how it went.
func (a *app) backupTo(dst string, done func(err error)) {
	if a.backupBusy {
		return
	}
	a.flush()
	a.backupBusy = true
	dir, skip := a.dir, a.cfg.BackupDir
	a.async(func() (any, error) {
		return nil, zipFolder(dir, dst, skip)
	}, func(_ any, err error) {
		a.backupBusy = false
		done(err)
	})
}

// backupNow asks where to save a backup and makes it.
func (a *app) backupNow() {
	a.plat.SaveFile(backupPrefix+a.now().Format("2006-01-02"), "zip", func(path string) {
		if path == "" {
			return
		}
		a.backupTo(path, func(err error) {
			if err != nil {
				a.saveErr = "备份失败：" + err.Error()
				return
			}
			a.tell("已备份到 " + filepath.Base(path))
		})
	})
}

// autoBackup makes the day's backup in the backups folder, once a day.
func (a *app) autoBackup() {
	dir := a.cfg.BackupDir
	now := a.now()
	if dir == "" || a.backupBusy || sameDay(a.cfg.BackupLast, now) {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.backupErr = err.Error()
		return
	}
	dst := filepath.Join(dir, backupPrefix+now.Format("2006-01-02")+".zip")
	a.cfg.BackupLast = now // tried today, whatever comes of it
	a.cfg.save()
	a.backupTo(dst, func(err error) {
		if err != nil {
			a.backupErr = err.Error()
			return
		}
		a.backupErr = ""
		pruneBackups(dir)
	})
}

func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// pruneBackups removes the oldest backups of a folder past backupsKept.
func pruneBackups(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), backupPrefix) && strings.HasSuffix(e.Name(), ".zip") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names) // by date, as named
	for len(names) > backupsKept {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}
