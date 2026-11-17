package main

import (
	"archive/zip"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Backups: the maps folder, with the library, versions of pictures and
// all, and the AI agents' skills, zipped into a file. Once a folder for them is chosen, a backup is
// made each day the app runs, and the last backupsKept are kept.

const (
	backupPrefix = "MyMind 备份 "
	backupsKept  = 14
)

// skillsInBackup is the folder of a backup that holds the AI agents'
// skills: they were in the maps folder's .skills, and so in every backup,
// until 0.100.7 moved them to ~/.agents/skills, which the sync mounts and
// the backup did not follow. Put back, they go to ~/.agents/skills.
const skillsInBackup = "技能（放回 .agents／skills）"

// zipFolder writes the files of dir, at any depth, into a zip file at
// dst, under dir's name; and those of the folders of more, each under the
// name it has there. Folders named in skip are left out.
func zipFolder(dir, dst string, more map[string]string, skip ...string) error {
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	files, err := zipInto(zw, dir, filepath.Base(dir), skip)
	if err == nil && files == 0 {
		err = errors.New("文件夹里没有可以备份的文件")
	}
	names := slices.Sorted(maps.Keys(more))
	for _, name := range names {
		if err == nil {
			_, err = zipInto(zw, more[name], name, skip)
		}
	}
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

// zipInto writes the files of dir into the zip under the folder name, and
// tells how many. A folder that is not there holds none.
func zipInto(zw *zip.Writer, dir, name string, skip []string) (files int, err error) {
	// A folder that is a link is walked where it leads: WalkDir does not
	// follow its root, and the backup of such a folder was an empty file
	// said to be done.
	root := dir
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		root = r
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // what cannot be read is left out
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		if d.IsDir() {
			// What the sync remembered of the folder (.sync, before 0.105.2)
			// is not of the backup: put back with it, files as they were
			// then read as changed in the repository alone, and the next
			// sync brought back what the backup was restored to be rid of.
			if path != root && (rel == ".sync" || slices.Contains(skip, filepath.Join(dir, rel)) || slices.Contains(skip, path)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || strings.HasSuffix(path, ".tmp") {
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
		h.Name = filepath.ToSlash(filepath.Join(name, rel))
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
		files++
		_, err = io.Copy(w, f)
		return err
	})
	return files, err
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
	// The AI agents' skills, which are kept out of the maps folder.
	more := map[string]string{}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		more[skillsInBackup] = skillsStore(home)
	}
	a.async(func() (any, error) {
		return nil, zipFolder(dir, dst, more, skip)
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
			a.tellAct("已备份到 "+filepath.Base(path), revealWord(), func() { a.plat.ShowInFolder(path) })
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
