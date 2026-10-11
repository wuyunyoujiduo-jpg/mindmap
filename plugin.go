package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/embed"
)

// The semantic search plugin: the embedding model, downloaded on demand
// from the public mindmap-models repository, through GitHub or, when
// GitHub is slow or blocked, the jsDelivr CDN that mirrors it. The model
// is split into parts under jsDelivr's size limit; a manifest gives their
// sizes and SHA-256, which every part and the whole are checked against.

// pluginSources are where the plugin's files are, tried in turn.
var pluginSources = []string{
	"https://raw.githubusercontent.com/wuyunyoujiduo-jpg/mindmap-models/main/embed/",
	"https://cdn.jsdelivr.net/gh/wuyunyoujiduo-jpg/mindmap-models@main/embed/",
	"https://fastly.jsdelivr.net/gh/wuyunyoujiduo-jpg/mindmap-models@main/embed/",
}

// pluginManifest describes the plugin's file and its parts.
type pluginManifest struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Parts   []struct {
		File   string `json:"file"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	} `json:"parts"`
}

// pluginDir is the app's folder of plugins.
func pluginDir() string {
	p := configPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "plugins")
}

// pluginPath is where the model plugin goes.
func pluginPath() string {
	if d := pluginDir(); d != "" {
		return filepath.Join(d, embed.FileName)
	}
	return ""
}

// installedManifest is the manifest of the plugin installed, nil for none.
func installedManifest() *pluginManifest {
	data, err := os.ReadFile(pluginPath() + ".json")
	if err != nil {
		return nil
	}
	var m pluginManifest
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	return &m
}

// pluginGetter fetches a URL's body; tests replace it.
type pluginGetter func(ctx context.Context, url string) ([]byte, error)

func httpGet(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s 返回 %d", req.URL.Host, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// fetchFrom gets a file of the plugin from the first source that serves
// it whole, as its SHA-256 says when one is given.
func fetchFrom(ctx context.Context, get pluginGetter, file, sum string) ([]byte, error) {
	var errs []string
	for _, src := range pluginSources {
		data, err := get(ctx, src+file)
		if err == nil && sum != "" && sha(data) != sum {
			err = errors.New("文件校验不符")
		}
		if err == nil {
			return data, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		errs = append(errs, err.Error())
	}
	return nil, fmt.Errorf("所有下载地址都失败了：%s", strings.Join(errs, "；"))
}

func sha(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// downloadPlugin downloads the model in the background, telling how far it
// got, and installs it.
func (a *app) downloadPlugin() {
	if a.pluginBusy {
		return
	}
	a.pluginBusy, a.pluginErr, a.pluginDone = true, "", 0
	ctx, cancel := context.WithCancel(context.Background())
	a.pluginCancel = cancel
	get, post := a.pluginGet, a.post
	progress := func(done int64) { post(func() { a.pluginDone = done }) }
	dst := pluginPath()
	a.async(func() (any, error) {
		defer cancel()
		data, err := fetchFrom(ctx, get, "manifest.json", "")
		if err != nil {
			return nil, err
		}
		var m pluginManifest
		if err := json.Unmarshal(data, &m); err != nil || len(m.Parts) == 0 {
			return nil, errors.New("插件清单无法读取")
		}
		post(func() { a.pluginSize = m.Size })
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		tmp, err := os.CreateTemp(filepath.Dir(dst), ".plugin-*.tmp")
		if err != nil {
			return nil, err
		}
		defer os.Remove(tmp.Name())
		h := sha256.New()
		var done int64
		for _, p := range m.Parts {
			part, err := fetchFrom(ctx, get, p.File, p.SHA256)
			if err != nil {
				tmp.Close()
				return nil, err
			}
			tmp.Write(part)
			h.Write(part)
			done += int64(len(part))
			progress(done)
		}
		if err := tmp.Close(); err != nil {
			return nil, err
		}
		if hex.EncodeToString(h.Sum(nil)) != m.SHA256 {
			return nil, errors.New("下载的文件校验不符，请重试")
		}
		if _, err := embed.Load(tmp.Name()); err != nil {
			return nil, fmt.Errorf("下载的模型无法读取：%w", err)
		}
		if err := os.Rename(tmp.Name(), dst); err != nil {
			return nil, err
		}
		meta, _ := json.Marshal(m)
		_ = os.WriteFile(dst+".json", meta, 0o644)
		return m, nil
	}, func(_ any, err error) {
		a.pluginBusy, a.pluginCancel = false, nil
		if err != nil {
			if errors.Is(err, context.Canceled) {
				err = errors.New("已取消")
			}
			a.pluginErr = err.Error()
			return
		}
		reloadLocalModel()
		a.embedErr, a.qvecs = "", map[string][]float32{}
		a.tell("语义检索插件已安装")
		a.embedLibrary()
	})
}

// removePlugin removes the model, and its clips' vectors with it.
func (a *app) removePlugin() {
	_ = os.Remove(pluginPath())
	_ = os.Remove(pluginPath() + ".json")
	reloadLocalModel()
	a.qvecs = map[string][]float32{}
}

// pluginInstalled tells whether the model plugin is there.
func pluginInstalled() bool {
	_, err := os.Stat(pluginPath())
	return err == nil
}

// pluginSettings is the plugin's part of the library's settings.
func (a *app) pluginSettings(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(6).Children(func() {
		switch {
		case a.pluginBusy:
			a.pluginProgress(c)
		case pluginInstalled():
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label := "✓ 语义检索插件已安装"
				if m := installedManifest(); m != nil {
					label += fmt.Sprintf("（版本 %d）", m.Version)
				}
				ui.Text(c, label).FontSize(12).TextColor(t.Success).Grow(1)
				if ui.Button(c, "检查更新").FontSize(12).Clicked() {
					a.checkPluginUpdate()
				}
				if ui.Button(c, "删除").FontSize(12).Tooltip("删除后资料库只按关键词搜索，可以随时再下载").Clicked() {
					a.removePlugin()
				}
			})
		case a.localModel() != nil:
			ui.Text(c, "✓ 正在使用应用自带的模型").FontSize(12).TextColor(t.Success)
		default:
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "语义检索插件：离线、免费、不上传内容，按意思搜资料并推荐相关资料。约 21MB，下载一次即可。").
					FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1)
				if ui.PrimaryButton(c, "下载插件").Clicked() {
					a.downloadPlugin()
				}
			})
		}
		if a.pluginErr != "" {
			ui.Text(c, "下载失败："+a.pluginErr).FontSize(12).TextColor(t.Danger).Wrap()
		}
		if a.pluginNote != "" {
			ui.Text(c, a.pluginNote).FontSize(12).TextColor(t.TextMuted).Wrap()
		}
	})
}

// pluginProgress shows how far the download got, with a way to stop it.
func (a *app) pluginProgress(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Spinner(c)
		label := "正在下载语义检索插件…"
		if a.pluginSize > 0 {
			label = fmt.Sprintf("正在下载语义检索插件… %.1f / %.1f MB", float64(a.pluginDone)/1e6, float64(a.pluginSize)/1e6)
			progressBar(c, float32(a.pluginDone)/float32(a.pluginSize), 120)
		}
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).Grow(1)
		if a.pluginCancel != nil && ui.Link(c, "取消", "").FontSize(12).Clicked() {
			a.pluginCancel()
		}
	})
}

// checkPluginUpdate downloads the plugin again when a newer one is out.
func (a *app) checkPluginUpdate() {
	have := installedManifest()
	get := a.pluginGet
	a.pluginNote = "正在检查…"
	a.async(func() (any, error) {
		data, err := fetchFrom(context.Background(), get, "manifest.json", "")
		if err != nil {
			return nil, err
		}
		var m pluginManifest
		return &m, json.Unmarshal(data, &m)
	}, func(v any, err error) {
		m, _ := v.(*pluginManifest)
		switch {
		case err != nil:
			a.pluginNote = "检查失败：" + err.Error()
		case have != nil && m.SHA256 == have.SHA256:
			a.pluginNote = "已是最新版本"
		default:
			a.pluginNote = ""
			a.downloadPlugin()
		}
	})
}

// pluginBanner offers the plugin in the library once, until downloaded or
// declined.
func (a *app) pluginBanner(c *ui.Context) {
	if a.cfg.PluginDeclined || a.cfg.NoEmbed || a.cfg.EmbedURL != "" || (a.localModel() != nil && !a.pluginBusy) || a.noLocalModel {
		return
	}
	t := c.Theme()
	ui.Row(c.Key("plugin-banner")).Padding(8, 12).Gap(10).AlignItems(ui.Center).Background(t.Accent.Alpha(0.08)).Children(func() {
		if a.pluginBusy {
			a.pluginProgress(c)
			return
		}
		ui.Text(c, "✦ 下载语义检索插件（约 21MB，离线免费），就能按意思搜资料，比如搜「数据库变慢」找到「SQL 优化」。").
			FontSize(12).Wrap().Grow(1)
		if a.pluginErr != "" {
			ui.Text(c, "下载失败，可以重试").FontSize(12).TextColor(t.Danger).Tooltip(a.pluginErr)
		}
		if ui.PrimaryButton(c, "下载").Clicked() {
			a.downloadPlugin()
		}
		if ui.Button(c, "不用了").Tooltip("以后可以在设置 → 资料库里下载").Clicked() {
			a.cfg.PluginDeclined = true
			a.cfg.save()
		}
	})
}
