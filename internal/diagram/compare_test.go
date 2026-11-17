package diagram

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// A comparison as the AI answers it: cells of their options by their
// place, what the map does not speak of left open however it was written,
// sources by the topics they stand for, and no row that judges.
func TestCompare(t *testing.T) {
	s, err := Parse(`好的：{"compare": {"options": ["GitHub 仓库", "", "iCloud 文件夹", "自己搭服务", "GitHub 仓库"],
	 "rows": [
	  {"dim": "历史版本", "cells": [{"text": "每一轮同步都是一个提交", "src": [5, 99]}, "空的方案", null, {"text": "要自己做", "src": 18}]},
	  {"dim": "用户要做什么", "cells": [{"text": "建一个私有仓库", "src": ["7"]}, {}, {"text": "放进 iCloud"}, {"text": "未提及", "src": [3]}]},
	  {"dim": "断网时", "cells": ["照常用", "x", "照常用", {"text": "照常用"}]},
	  {"dim": "费用", "same": "true", "cells": [{"text": "免费"}, {}, {"text": "不花钱"}, {"text": "不要钱"}]},
	  {"dim": "另一台不是 Mac", "same": true, "cells": [{"text": "能用"}, {}, {"gap": true, "text": "大概不能"}, {"text": "能用"}]},
	  {"dim": "综合推荐", "cells": [{"text": "首选"}, {}, {"text": "其次"}, {"text": "不建议"}]},
	  {"dim": "结论", "cells": ["最好", "", "一般", "差"]},
	  {"dim": "没人说", "cells": [null, null, null, null]},
	  {"dim": "历史版本", "cells": ["重复的一行", "", "x", "y"]}]},
	 "verdict": "GitHub 仓库最好", "lean": "GitHub 仓库"}`)
	if err != nil {
		t.Fatal(err)
	}
	s.Clean(func(v string) (string, bool) { return "t" + v, v != "99" })
	c := s.Compare
	if c == nil || !slices.Equal(c.Options, []string{"GitHub 仓库", "iCloud 文件夹", "自己搭服务"}) || !slices.Equal(s.Kinds(), []string{"compare"}) {
		t.Fatalf("options %+v, kinds %v", c, s.Kinds())
	}
	var dims []string
	for _, r := range c.Rows {
		dims = append(dims, r.Dim)
		if len(r.Cells) != 3 {
			t.Fatalf("%s has %d cells", r.Dim, len(r.Cells))
		}
	}
	// The rows that judge, the one of nothing said and the one said twice are gone.
	if !slices.Equal(dims, []string{"历史版本", "用户要做什么", "断网时", "费用", "另一台不是 Mac"}) {
		t.Fatalf("rows %v", dims)
	}
	// A cell is of its option though one before it was dropped; unsaid has
	// neither words nor sources.
	h := c.Rows[0].Cells
	if h[0].Text != "每一轮同步都是一个提交" || !slices.Equal(h[0].Src, []string{"t5"}) || !h[1].Gap || h[1].Text != "" ||
		h[2].Text != "要自己做" || !slices.Equal(h[2].Src, []string{"t18"}) {
		t.Fatalf("cells %+v", h)
	}
	if u := c.Rows[1].Cells; !u[2].Gap || u[2].Text != "" || u[2].Src != nil || u[1].Text != "放进 iCloud" {
		t.Fatalf("a cell said to be unsaid in words: %+v", u)
	}
	if m := c.Rows[4].Cells[1]; !m.Gap || m.Text != "" {
		t.Fatalf("a gap kept the AI's guess: %+v", m)
	}
	// The same: by the words, or as the answer says; never with one unsaid.
	if c.Rows[0].Same || !c.Rows[2].Same || !c.Rows[3].Same || c.Rows[4].Same || c.Sames() != 2 || c.Gaps() != 3 {
		t.Fatalf("same %v %v %v %v, %d the same, %d unsaid", c.Rows[0].Same, c.Rows[2].Same, c.Rows[3].Same, c.Rows[4].Same, c.Sames(), c.Gaps())
	}
	// What the user leans to is no answer's.
	if s.Lean != "" {
		t.Fatalf("the AI leant to %q", s.Lean)
	}

	// The user's mark stays while its option does, and the column last
	// asked back while there is one.
	s.Lean, s.CompareQuiz = "iCloud 文件夹", &Quiz{Col: 3}
	s.Clean(func(v string) (string, bool) { return v, true })
	if s.Lean != "iCloud 文件夹" || s.CompareQuiz == nil {
		t.Fatal("the mark went though its option is there")
	}
	c.Options[1] = "坚果云"
	s.Clean(func(v string) (string, bool) { return v, true })
	if s.Lean != "" {
		t.Fatal("the mark stayed on an option that is gone")
	}
	s.Lean = "坚果云"
	md := s.Markdown()
	if !strings.Contains(md, "## 对比") || !strings.Contains(md, "| 历史版本 | 每一轮同步都是一个提交 | 导图里没说 | 要自己做 |") || !strings.Contains(md, "我的倾向：坚果云") {
		t.Fatalf("markdown:\n%s", md)
	}
	// Pasted elsewhere, its cells name the topics there.
	if r := s.Renamed(map[string]string{"t5": "n5"}); r.Compare.Rows[0].Cells[0].Src[0] != "n5" || s.Compare.Rows[0].Cells[0].Src[0] != "t5" {
		t.Fatalf("renamed %+v", r.Compare.Rows[0].Cells[0])
	}

	// A version before this kind reads the set without it and does not
	// fail: its other kinds are there, the comparison is not.
	s.Objects = []Object{{Name: "令牌"}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var old struct {
		Table   *Table   `json:"table,omitempty"`
		Objects []Object `json:"objects,omitempty"`
	}
	if err := json.Unmarshal(data, &old); err != nil || len(old.Objects) != 1 || old.Table != nil {
		t.Fatalf("an older version reads %+v, %v", old, err)
	}

	// Too little to compare is not kept: one option, no row but one that
	// judges, or not a table at all.
	for _, text := range []string{
		`{"compare": {"options": ["只有一个"], "rows": [{"dim": "费用", "cells": ["免费"]}]}}`,
		`{"compare": {"options": ["甲", "乙"], "rows": [{"dim": "哪个更好", "cells": ["甲", "乙"]}]}}`,
		`{"compare": [{"options": ["甲", "乙"]}]}`,
	} {
		s, err := Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		if s.Clean(func(v string) (string, bool) { return v, true }); !s.Empty() {
			t.Fatalf("%s kept: %+v", text, s.Compare)
		}
	}
	for dim, judges := range map[string]bool{"推荐": true, "综合评分": true, "谁更合适": true, "总结": true, "费用": false, "历史版本": false, "用户要做什么": false} {
		if Verdict(dim) != judges {
			t.Fatalf("Verdict(%q) = %v", dim, !judges)
		}
	}
}
