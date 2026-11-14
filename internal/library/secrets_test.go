package library

import (
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	for _, c := range []struct{ in, gone, stays string }{
		{"用这个 sk-abcdefghijklmnopqrstuvwx 调接口", "abcdefghijkl", "调接口"},
		{"RELEASES_TOKEN=github_pat_11ABCDEFG0abcdefghijklmn_opqrstuvwxyz", "github_pat_", "RELEASES_TOKEN="},
		{"MYGO_UPDATER_PRIVATE_KEY=dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWdu", "dW50cnVzdGVk", "MYGO_UPDATER_PRIVATE_KEY="},
		{"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n后面的话", "b3BlbnNzaC1rZXk", "后面的话"},
		{`{"api_key": "9f8e7d6c5b4a3210ffee"}`, "9f8e7d6c5b4a", `"api_key": "`},
		{"数据库密码：Hunter2Hunter2，别外传", "Hunter2", "别外传"},
		{"ghp_abcdefghijklmnopqrstuvwxyz0123456789", "abcdefghij", ""},
	} {
		got := Mask(c.in)
		if strings.Contains(got, c.gone) || !strings.Contains(got, c.stays) || !strings.Contains(got, masked) || !HasSecret(c.in) {
			t.Errorf("%q → %q", c.in, got)
		}
	}
	for _, plain := range []string{"写周报", "keyboard 的快捷键很好用", "token 这个词本身没什么", "密码要定期换", "sk-short"} {
		if got := Mask(plain); got != plain || HasSecret(plain) {
			t.Errorf("%q → %q", plain, got)
		}
	}
}
