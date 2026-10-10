package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/formats"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Other formats in and out: Markdown and OPML outlines, a calendar of the
// tasks, the slides as a PDF; and a repository's GitHub issues as tasks.

// askImportOutline asks for a Markdown or OPML file and makes a map of it.
func (a *app) askImportOutline() {
	a.plat.OpenFile("导入 Markdown 或 OPML", []string{"md", "markdown", "txt", "opml"}, func(path string) {
		if path != "" {
			a.importOutline(path)
		}
	})
}

func (a *app) importOutline(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		a.saveErr = "导入失败: " + err.Error()
		return
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var f *model.File
	if strings.EqualFold(filepath.Ext(path), ".opml") {
		f, err = formats.ParseOPML(data, name)
		if err != nil {
			a.saveErr = "导入失败: " + err.Error()
			return
		}
	} else {
		f = formats.ParseMarkdown(string(data), name)
	}
	dest := freeMapPath(a.currentFolder(), name)
	if err := f.Save(dest); err != nil {
		a.saveErr = "导入失败: " + err.Error()
		return
	}
	a.refreshFiles()
	_ = a.open(dest)
}

func (a *app) askExportOPML() {
	a.exportTo("opml", func(path string) error {
		data, err := formats.OPML(a.file, mapName(a.path))
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	})
}

// askExportICS saves the tasks with dates as a calendar, which Calendar
// and other apps import.
func (a *app) askExportICS() {
	a.exportTo("ics", func(path string) error {
		return os.WriteFile(path, formats.ICS(a.file, mapName(a.path), a.now()), 0o644)
	})
}

// askExportSlides saves the slides of the canvas shown as a PDF, each
// slide with all it shows.
func (a *app) askExportSlides() {
	a.exportTo("pdf", func(path string) error {
		var buf bytes.Buffer
		if err := a.slidesPDF(&buf); err != nil {
			return err
		}
		return os.WriteFile(path, buf.Bytes(), 0o644)
	})
}

func (a *app) slidesPDF(buf *bytes.Buffer) (err error) {
	slides := slidesOf(a.doc)
	if len(slides) == 0 {
		return errors.New("这个画布没有可生成的幻灯片")
	}
	w, h := float32(1280), float32(720)
	switch a.doc.Present.Aspect {
	case "4:3":
		w, h = 1024, 768
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	var pages []image.Image
	for i, s := range slides {
		s := s
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().Children(func() {
				a.slideView(c, a.doc, s, i, len(slides), w, h, -1) // all of it shown
			})
		}, int(w), int(h))
		tt.SetScale(2)
		tt.Frame()
		pages = append(pages, tt.Image())
	}
	return formats.PDF(buf, pages, float64(w)*0.75) // points: 3/4 of a pixel
}

// GitHub issues as tasks: a map names its repository; syncing adds the
// open issues as tasks under a "GitHub Issues" branch, each linked to its
// page, and marks done those closed.

type ghIssue struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	State       string `json:"state"`
	HTMLURL     string `json:"html_url"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// fetchIssues reads a repository's issues, open and closed, without pull
// requests.
func fetchIssues(ctx context.Context, api, repo, token string) ([]ghIssue, error) {
	var all []ghIssue
	client := &http.Client{Timeout: 30 * time.Second}
	for page := 1; page <= 5; page++ {
		url := fmt.Sprintf("%s/repos/%s/issues?state=all&per_page=100&page=%d", strings.TrimRight(api, "/"), repo, page)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("连接 GitHub 失败：%w", err)
		}
		var issues []ghIssue
		err = json.NewDecoder(resp.Body).Decode(&issues)
		resp.Body.Close()
		switch {
		case resp.StatusCode == 404:
			return nil, fmt.Errorf("找不到仓库 %s（私有仓库需要在设置里填 GitHub Token）", repo)
		case resp.StatusCode == 401 || resp.StatusCode == 403:
			return nil, fmt.Errorf("GitHub 拒绝访问（%d），检查设置里的 Token", resp.StatusCode)
		case resp.StatusCode != 200:
			return nil, fmt.Errorf("GitHub 返回 %d", resp.StatusCode)
		case err != nil:
			return nil, err
		}
		for _, is := range issues {
			if is.PullRequest == nil {
				all = append(all, is)
			}
		}
		if len(issues) < 100 {
			break
		}
	}
	return all, nil
}

// syncIssues brings the issues into the map: new open ones are added,
// those there follow their title and state.
func syncIssues(f *model.File, issues []ghIssue, now time.Time) (added, updated int) {
	byURL := map[string]*model.Node{}
	f.Walk(func(n *model.Node) {
		for _, l := range n.Links {
			if strings.Contains(l, "/issues/") {
				byURL[l] = n
			}
		}
	})
	d := f.Sheets[0]
	var box *model.Node
	for _, c := range d.Root.Children {
		if c.Title == "GitHub Issues" {
			box = c
		}
	}
	for _, is := range slices.Backward(issues) { // the oldest first
		title := fmt.Sprintf("#%d %s", is.Number, is.Title)
		status := model.StatusTodo
		if is.State == "closed" {
			status = model.StatusDone
		}
		if n, ok := byURL[is.HTMLURL]; ok {
			changed := false
			if n.Title != title {
				n.Title, changed = title, true
			}
			if n.Task == nil {
				n.Task = &model.Task{}
			}
			if status == model.StatusDone && n.Task.Status != model.StatusDone ||
				status == model.StatusTodo && n.Task.Status == model.StatusDone {
				n.Task.SetStatus(status, now)
				changed = true
			}
			if changed {
				updated++
			}
			continue
		}
		if is.State == "closed" {
			continue // only open ones are added
		}
		if box == nil {
			box = model.NewNode("GitHub Issues")
			d.Root.Children = append(d.Root.Children, box)
		}
		n := model.NewNode(title)
		n.Task = &model.Task{Status: model.StatusTodo}
		n.Links = []string{is.HTMLURL}
		body := strings.TrimSpace(is.Body)
		if r := []rune(body); len(r) > 500 {
			body = string(r[:500]) + "…"
		}
		n.Note = body
		for _, l := range is.Labels {
			n.Tags = append(n.Tags, l.Name)
		}
		box.Children = append(box.Children, n)
		added++
	}
	return added, updated
}

// syncGitHub syncs the map's repository's issues in the background.
func (a *app) syncGitHub() {
	repo := strings.Trim(strings.TrimSpace(a.file.GitHub), "/")
	repo = strings.TrimPrefix(repo, "https://github.com/")
	if strings.Count(repo, "/") != 1 {
		a.saveErr = "GitHub 仓库的格式应为 owner/repo"
		return
	}
	if a.ghBusy {
		return
	}
	a.ghBusy = true
	api, token, path := a.cfg.GitHubAPI, a.cfg.GitHubToken, a.path
	if api == "" {
		api = "https://api.github.com"
	}
	a.async(func() (any, error) {
		return fetchIssues(context.Background(), api, repo, token)
	}, func(v any, err error) {
		a.ghBusy = false
		if err != nil {
			a.ghResult = err.Error()
			return
		}
		if a.path != path {
			return
		}
		issues := v.([]ghIssue)
		var added, updated int
		a.mutate("", func() { added, updated = syncIssues(a.file, issues, a.now()) })
		a.ghResult = fmt.Sprintf("%s 同步完成：新增 %d，更新 %d（共 %d 个 issue）", a.now().Format("15:04"), added, updated, len(issues))
		a.tell(a.ghResult)
	})
}

// githubSection is the map's repository, to sync its issues.
func (a *app) githubSection(c *ui.Context) {
	t := c.Theme()
	ui.Field(c, "GitHub 仓库", func() {
		ui.Column(c).Gap(6).Children(func() {
			repo := a.file.GitHub
			if ui.TextInput(c.Key("gh-repo"), &repo).Placeholder("owner/repo").Label("GitHub 仓库").Changed() {
				a.mutate("github", func() { a.file.GitHub = strings.TrimSpace(repo) })
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label := "同步 Issues"
				if a.ghBusy {
					label = "同步中…"
				}
				if ui.Button(c, label).FontSize(12).Disabled(a.ghBusy || a.file.GitHub == "").Clicked() {
					a.syncGitHub()
				}
			})
			if a.ghResult != "" {
				ui.Text(c, a.ghResult).FontSize(11).TextColor(t.TextMuted).Wrap()
			}
			ui.Text(c, "打开的 issue 作为待做任务加到「GitHub Issues」分支下并链接到 issue；再次同步会更新标题，关闭的标为完成。私有仓库在设置里填 Token。").
				FontSize(11).TextColor(t.TextMuted).Wrap()
		})
	})
}
