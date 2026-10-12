package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnalysis(t *testing.T) {
	f := NewFile("项目")
	child := NewNode("登录")
	child.Note = "支持扫码"
	child.Tags = []string{"核心"}
	f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, child)

	h := f.ContentHash()
	if out := f.Outline(1000); !strings.Contains(out, "- 登录 #核心") || !strings.Contains(out, "备注：支持扫码") {
		t.Fatalf("outline %q", out)
	}
	f.Analysis = &Analysis{Summary: "一个登录项目", Synonyms: []string{"认证", "SSO"}, Hash: h}
	if f.ContentHash() != h {
		t.Fatal("the analysis changed the content's hash")
	}
	child.Note = "支持扫码和短信"
	if f.ContentHash() == h {
		t.Fatal("a note's change kept the hash")
	}
	if !f.Analysis.Matches("sso") || f.Analysis.Matches("支付") || (*Analysis)(nil).Matches("x") {
		t.Fatal("matches")
	}
	if c := f.Clone(); c.Analysis != f.Analysis {
		t.Fatal("clone lost the analysis")
	}
	data, _ := json.Marshal(f)
	var back File
	if err := json.Unmarshal(data, &back); err != nil || back.Analysis == nil || back.Analysis.Summary != "一个登录项目" {
		t.Fatalf("round trip %v %+v", err, back.Analysis)
	}
	if r := []rune(f.Outline(5)); len(r) > 20 {
		t.Fatalf("limit not kept: %q", string(r))
	}
}
