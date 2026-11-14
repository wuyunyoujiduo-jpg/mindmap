package gitbase

import "testing"

func TestNormalRemote(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:Me/Shop.git":                  "github.com/me/shop",
		"https://github.com/me/shop":                  "github.com/me/shop",
		"https://token@github.com/me/shop.git/":       "github.com/me/shop",
		"ssh://git@gitlab.example.com:2222/g/s/p.git": "gitlab.example.com/g/s/p",
		"/Users/me/code/shop":                         "",
		"":                                            "",
	} {
		if got := NormalRemote(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
