package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Each map keeps how it was left: the canvas shown, the view, where each
// canvas was looked at from and the node selected on it, and, while the app
// runs, its undo history. Switching back to a map, or opening the app
// again, shows it as it was. This is kept apart from the maps, in the
// app's own folder, so their files change only when they do.

// camState is a canvas's camera as remembered.
type camState struct {
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	Zoom     float32 `json:"zoom"`
	Selected string  `json:"selected,omitempty"`
}

// fileView is how a map was left.
type fileView struct {
	Sheet string              `json:"sheet"`
	Mode  int                 `json:"mode"`
	Cams  map[string]camState `json:"cams"`
}

func viewsPath() string {
	p := configPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "views.json")
}

func loadViews() map[string]*fileView {
	views := map[string]*fileView{}
	if p := viewsPath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &views)
		}
	}
	return views
}

func (a *app) saveViews() {
	p := viewsPath()
	if p == "" {
		return
	}
	// Maps that are gone are forgotten.
	for path := range a.views {
		if _, err := os.Stat(path); err != nil {
			delete(a.views, path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	b, _ := json.MarshalIndent(a.views, "", "  ")
	_ = os.WriteFile(p, b, 0o644)
}

// rememberView keeps how the open map is left, and its history.
func (a *app) rememberView() {
	if a.file == nil || a.path == "" || a.doc == nil {
		return
	}
	a.cam().selected = a.selected
	v := &fileView{Sheet: a.doc.ID, Mode: a.mode, Cams: map[string]camState{}}
	for id, c := range a.cams {
		if c.placed {
			v.Cams[id] = camState{X: c.X, Y: c.Y, Zoom: c.Zoom, Selected: c.selected}
		} else if c.selected != "" {
			v.Cams[id] = camState{Zoom: c.Zoom, Selected: c.selected} // placed anew
		}
	}
	a.views[a.path] = v
	a.hists[a.path] = a.hist
}

// restoreView shows the map just opened as it was left: its canvas, view,
// cameras and history. A map never opened starts on its first canvas.
func (a *app) restoreView(f *model.File) {
	a.cams = map[string]*camera{}
	a.hist = a.hists[a.path]
	if a.hist == nil {
		a.hist = model.NewHistory(200)
	}
	sheet := f.Sheets[0].ID
	if v := a.views[a.path]; v != nil {
		for id, c := range v.Cams {
			if d, _ := f.Sheet(id); d == nil {
				continue
			}
			z := c.Zoom
			if z <= 0 {
				z = 1
			}
			a.cams[id] = &camera{X: c.X, Y: c.Y, Zoom: z, placed: c.X != 0 || c.Y != 0, selected: c.Selected}
		}
		if d, _ := f.Sheet(v.Sheet); d != nil {
			sheet = v.Sheet
		}
		if v.Mode >= viewMap && v.Mode <= viewOverview {
			a.mode = v.Mode
		}
	}
	a.doc = nil
	a.showSheet(sheet)
}

// renameViews follows a map, or a folder of maps, to its new path.
func (a *app) renameViews(follow func(string) string) {
	views, hists := map[string]*fileView{}, map[string]*model.History{}
	for p, v := range a.views {
		views[follow(p)] = v
	}
	for p, h := range a.hists {
		hists[follow(p)] = h
	}
	a.views, a.hists = views, hists
}
