package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Ext is the extension of mind map files.
const Ext = ".mindmap.json"

// Load reads a mind map file.
func Load(path string) (*Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes a mind map file's contents.
func Parse(b []byte) (*Document, error) {
	var d Document
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("文件格式错误: %w", err)
	}
	if d.Root == nil {
		return nil, errors.New("文件格式错误: 缺少根节点")
	}
	if d.Version > FormatVersion {
		return nil, fmt.Errorf("文件版本 %d 比本程序支持的 %d 新", d.Version, FormatVersion)
	}
	d.Version = FormatVersion
	seen := map[string]bool{}
	d.Walk(func(n, _ *Node, _ int) bool {
		if n.ID == "" || seen[n.ID] {
			n.ID = NewID()
		}
		seen[n.ID] = true
		return true
	})
	return &d, nil
}

// Marshal encodes the document as indented JSON, which diffs well in git.
func (d *Document) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Save writes the document to path atomically: through a temporary file
// renamed over it, so a crash never leaves half a file.
func (d *Document) Save(path string) error {
	b, err := d.Marshal()
	if err != nil {
		return err
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
