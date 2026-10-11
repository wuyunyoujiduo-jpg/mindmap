package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// TestStress times frames of every view on a large map and a large
// library, when MINDMAP_STRESS is set: a map of about 3000 nodes, many of
// them tasks, and a library of 3000 clips.
func TestStress(t *testing.T) {
	if os.Getenv("MINDMAP_STRESS") == "" {
		t.Skip("set MINDMAP_STRESS to run")
	}
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	// The map: 15 branches of 10 of 20.
	a.mutate("", func() {
		root := a.doc.Root
		for i := range 15 {
			b := model.NewNode(fmt.Sprintf("模块 %d", i))
			for j := range 10 {
				c := model.NewNode(fmt.Sprintf("功能 %d-%d：一个稍长一点的标题", i, j))
				c.Note = "实现要点：先写接口，再补测试。"
				for k := range 20 {
					n := model.NewNode(fmt.Sprintf("任务 %d-%d-%d", i, j, k))
					due := testNow.AddDate(0, 0, k-10)
					n.Task = &model.Task{Status: model.Statuses[k%len(model.Statuses)], Due: &due}
					n.Tags = []string{fmt.Sprintf("标签%d", k%8)}
					c.Children = append(c.Children, n)
				}
				b.Children = append(b.Children, c)
			}
			root.Children = append(root.Children, b)
		}
	})
	time1 := func(name string, frames int) {
		start := time.Now()
		for range frames {
			tt.Frame()
		}
		t.Logf("%-12s %6.1f ms a frame", name, float64(time.Since(start).Microseconds())/1000/float64(frames))
	}
	if os.Getenv("MINDMAP_STRESS") == "map" {
		a.mode = viewMap
		tt.Frame()
		time1("导图", 20)
		return
	}
	if os.Getenv("MINDMAP_STRESS") == "board" {
		a.mode = viewBoard
		tt.Frame()
		time1("看板", 20)
		return
	}
	for _, v := range []struct {
		name string
		mode int
	}{{"导图", viewMap}, {"大纲", viewOutline}, {"看板", viewBoard}, {"日程", viewAgenda}, {"时间线", viewTimeline}, {"总览", viewOverview}} {
		a.mode = v.mode
		tt.Frame()
		time1(v.name, 10)
	}
	a.mode = viewMap
	a.search = "任务 3"
	time1("导图·搜索", 10)
	a.search = ""

	// The library.
	start := time.Now()
	for i := range 3000 {
		c := &library.Clip{Kind: library.KindText, Title: fmt.Sprintf("资料 %d", i),
			Text:     fmt.Sprintf("第 %d 条资料，讲缓存、数据库索引、前端性能和团队管理里的某个细节。%s", i, longText),
			Category: []string{"技术", "产品", "学习", "生活"}[i%4], Tags: []string{fmt.Sprintf("t%d", i%30)},
			Desc: "一条用来压测的资料。", Synonyms: []string{"压测", "性能"}}
		if err := a.lib.Add(c, testNow.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("adding 3000 clips: %v", time.Since(start))
	a.showLibrary()
	tt.Frame()
	time1("资料库", 10)
	a.libQuery = "数据库索引"
	time1("资料库·搜索", 10)
	a.libQuery, a.libSel = "", a.lib.Clips[0].ID
	time1("资料详情", 10)
	start = time.Now()
	l, _ := library.Open(a.lib.Dir)
	t.Logf("opening a library of %d clips: %v", len(l.Clips), time.Since(start))
}

var longText = func() string {
	s := ""
	for range 40 {
		s += "这是一段用来压测的正文，包含一些常见的词，比如缓存、索引、接口、测试、发布。"
	}
	return s
}()
