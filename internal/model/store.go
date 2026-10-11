package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Ext is the extension of mind map files.
const Ext = ".mindmap.json"

// AssetsExt ends the name of the folder of a map's images, beside it:
// "计划.mindmap.json" keeps them in "计划.assets".
const AssetsExt = ".assets"

// AssetsDir is the folder of the images of the map in path.
func AssetsDir(path string) string {
	return strings.TrimSuffix(path, Ext) + AssetsExt
}

// Load reads a mind map file.
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes a mind map file's contents. A file of version 1, a single
// map, becomes a file of one canvas.
func Parse(b []byte) (*File, error) {
	var raw struct {
		Version int         `json:"version"`
		Root    *Node       `json:"root"` // version 1
		Sheets  []*Document `json:"sheets"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("文件格式错误: %w", err)
	}
	if raw.Version > FormatVersion {
		return nil, fmt.Errorf("文件版本 %d 比本程序支持的 %d 新", raw.Version, FormatVersion)
	}
	f := &File{Version: FormatVersion, Sheets: raw.Sheets}
	if len(f.Sheets) == 0 && raw.Root != nil {
		f.Sheets = []*Document{{Title: "画布 1", Root: raw.Root}}
	}
	if len(f.Sheets) == 0 {
		return nil, errors.New("文件格式错误: 没有画布")
	}
	sheets, nodes := map[string]bool{}, map[string]bool{}
	for i, d := range f.Sheets {
		if d == nil || d.Root == nil {
			return nil, errors.New("文件格式错误: 画布缺少根节点")
		}
		if d.ID == "" || sheets[d.ID] {
			d.ID = NewID()
		}
		sheets[d.ID] = true
		if d.Title == "" {
			d.Title = fmt.Sprintf("画布 %d", i+1)
		}
		if !slices.Contains(Structures, d.Structure) {
			d.Structure = StructureMindMap // one a newer version added
		}
		d.Walk(func(n, _ *Node, _ int) bool {
			if n.ID == "" || nodes[n.ID] {
				n.ID = NewID()
			}
			nodes[n.ID] = true
			return true
		})
	}
	return f, nil
}

// Marshal encodes the file as indented JSON, which diffs well in git.
func (f *File) Marshal() ([]byte, error) {
	f.Version = FormatVersion
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Save writes the file to path atomically: through a temporary file
// renamed over it, so a crash never leaves half a file.
func (f *File) Save(path string) error {
	b, err := f.Marshal()
	if err != nil {
		return err
	}
	// What is written must read back: a file that would not open is
	// never put in place of one that does.
	if _, err := Parse(b); err != nil {
		return fmt.Errorf("保存前校验失败，文件未改动：%w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mindmap-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// FileInfo describes a mind map file of a folder.
type FileInfo struct {
	Path     string
	Name     string // without the extension
	Modified time.Time
}

// ListFiles returns the mind map files directly in dir, most recently
// modified first.
func ListFiles(dir string) ([]FileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []FileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), Ext) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Path:     filepath.Join(dir, e.Name()),
			Name:     strings.TrimSuffix(e.Name(), Ext),
			Modified: info.ModTime(),
		})
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Modified.After(files[j].Modified) })
	return files, nil
}

// FileName returns the file name for a map named name, with characters
// file systems refuse replaced.
func FileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, name)
	if name == "" {
		name = "未命名"
	}
	return name + Ext
}

// Folder is a folder of mind map files, with the folders in it.
type Folder struct {
	Path    string
	Name    string
	Folders []Folder
	Files   []FileInfo
}

// MaxFolderDepth is how deep ScanFolder looks for folders.
const MaxFolderDepth = 4

// ScanFolder returns dir's mind maps and those of the folders in it, down
// to MaxFolderDepth, leaving out hidden folders. Folders are sorted by
// name, files most recently modified first.
func ScanFolder(dir string) (Folder, error) {
	return scanFolder(dir, 0)
}

func scanFolder(dir string, depth int) (Folder, error) {
	f := Folder{Path: dir, Name: filepath.Base(dir)}
	files, err := ListFiles(dir)
	if err != nil {
		return f, err
	}
	f.Files = files
	if depth >= MaxFolderDepth {
		return f, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return f, err
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasSuffix(e.Name(), AssetsExt) {
			continue
		}
		sub, err := scanFolder(filepath.Join(dir, e.Name()), depth+1)
		if err == nil {
			f.Folders = append(f.Folders, sub)
		}
	}
	sort.SliceStable(f.Folders, func(i, j int) bool { return f.Folders[i].Name < f.Folders[j].Name })
	return f, nil
}

// AllFiles returns the files of the folder and of every folder in it.
func (f Folder) AllFiles() []FileInfo {
	files := append([]FileInfo(nil), f.Files...)
	for _, sub := range f.Folders {
		files = append(files, sub.AllFiles()...)
	}
	return files
}

// FolderPaths returns the paths of the folder and every folder in it.
func (f Folder) FolderPaths() []string {
	paths := []string{f.Path}
	for _, sub := range f.Folders {
		paths = append(paths, sub.FolderPaths()...)
	}
	return paths
}

// MoveFile moves a mind map file into the folder dir, keeping its name,
// and returns its new path. It does not replace a file of that name.
func MoveFile(path, dir string) (string, error) {
	to := filepath.Join(dir, filepath.Base(path))
	if to == path {
		return path, nil
	}
	if _, err := os.Stat(to); err == nil {
		return "", fmt.Errorf("「%s」中已有同名导图", filepath.Base(dir))
	}
	if err := os.Rename(path, to); err != nil {
		return "", err
	}
	MoveAssets(path, to)
	return to, nil
}

// MoveAssets moves the images of the map at from with it to to, when it
// has any.
func MoveAssets(from, to string) {
	if _, err := os.Stat(AssetsDir(from)); err == nil {
		_ = os.Rename(AssetsDir(from), AssetsDir(to))
	}
}

// AllFilesIn returns the paths of every map in dir and the folders in it.
func AllFilesIn(dir string) ([]string, error) {
	f, err := ScanFolder(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, info := range f.AllFiles() {
		paths = append(paths, info.Path)
	}
	return paths, nil
}
