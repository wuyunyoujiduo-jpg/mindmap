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
	an := &Analysis{Summary: "一个登录项目", Synonyms: []string{"认证", "SSO"}, Hash: h}
	child.Note = "支持扫码和短信"
	if f.ContentHash() == h {
		t.Fatal("a note's change kept the hash")
	}
	if !an.Matches("sso") || an.Matches("支付") || (*Analysis)(nil).Matches("x") {
		t.Fatal("matches")
	}
	data, _ := json.Marshal(an)
	var back Analysis
	if err := json.Unmarshal(data, &back); err != nil || back.Summary != "一个登录项目" {
		t.Fatalf("round trip %v %+v", err, back)
	}
	if r := []rune(f.Outline(5)); len(r) > 20 {
		t.Fatalf("limit not kept: %q", string(r))
	}
}
