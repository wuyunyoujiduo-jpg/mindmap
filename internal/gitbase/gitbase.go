// Package gitbase reads where a project's code is, in git: the commit a
// map was last brought up to date with, and what changed since, so that
// an AI updating the map reads the changes, not all the code again.
package gitbase

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Head is the commit checked out in a repository.
type Head struct {
	Branch  string // "" when detached
	Commit  string // the full hash
	Subject string // the commit message's first line
}

// IsRepo tells whether dir is in a git repository, without running git
// (on a Mac without the developer tools, running it asks to install them).
func IsRepo(dir string) bool {
	if dir == "" {
		return false
	}
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		if filepath.Dir(d) == d {
			return false
		}
	}
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// HeadOf reads the commit checked out in dir.
func HeadOf(dir string) (Head, error) {
	if !IsRepo(dir) {
		return Head{}, fmt.Errorf("%s 不是 git 仓库", dir)
	}
	out, err := git(dir, "log", "-1", "--format=%H%x00%s")
	if err != nil {
		return Head{}, err
	}
	commit, subject, _ := strings.Cut(out, "\x00")
	h := Head{Commit: commit, Subject: subject}
	if b, err := git(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && b != "HEAD" {
		h.Branch = b
	}
	return h, nil
}

// Resolve gives the full hash of a commit, a branch or a tag.
func Resolve(dir, rev string) (string, error) {
	return git(dir, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
}

// Change is a file changed since a commit.
type Change struct {
	Status string // A, M, D, R…
	Path   string // relative to the repository's root, with slashes
	From   string // a renamed file's old path
}

// Since lists the commits since base, newest first, as "hash subject"
// lines, and the files changed from base to the working tree (what is
// not committed yet too).
func Since(dir, base string) (commits []string, changes []Change, err error) {
	if !IsRepo(dir) {
		return nil, nil, fmt.Errorf("%s 不是 git 仓库", dir)
	}
	if _, err := Resolve(dir, base); err != nil {
		return nil, nil, fmt.Errorf("仓库里没有 commit %s（可能还没拉取，或历史被改写）", short(base))
	}
	log, err := git(dir, "log", "--format=%h %s", base+"..HEAD")
	if err != nil {
		return nil, nil, err
	}
	for _, l := range strings.Split(log, "\n") {
		if l != "" {
			commits = append(commits, l)
		}
	}
	diff, err := git(dir, "diff", "--name-status", "-M", base)
	if err != nil {
		return nil, nil, err
	}
	for _, l := range strings.Split(diff, "\n") {
		f := strings.Split(l, "\t")
		if len(f) < 2 || f[0] == "" {
			continue
		}
		c := Change{Status: f[0][:1], Path: f[len(f)-1]}
		if len(f) == 3 {
			c.From = f[1]
		}
		changes = append(changes, c)
	}
	// Files git doesn't know yet are new too.
	if untracked, err := git(dir, "ls-files", "--others", "--exclude-standard"); err == nil {
		for _, p := range strings.Split(untracked, "\n") {
			if p != "" {
				changes = append(changes, Change{Status: "A", Path: p})
			}
		}
	}
	return commits, changes, nil
}

// Root is the repository's top folder, which git's paths are relative to.
func Root(dir string) (string, error) { return git(dir, "rev-parse", "--show-toplevel") }

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// Short is a commit's hash as people write it.
func Short(commit string) string { return short(commit) }

// Patch is what changed in the code from base to the working tree, as a
// diff of at most max bytes; lock files and the like are left out.
func Patch(dir, base string, max int) (string, error) {
	out, err := git(dir, "diff", "-M", "--no-color", base, "--", ".",
		":(exclude)*.lock", ":(exclude)go.sum", ":(exclude)package-lock.json", ":(exclude)pnpm-lock.yaml",
		":(exclude)*.min.js", ":(exclude)*.svg", ":(exclude)*.png", ":(exclude)*.jpg")
	if err != nil {
		return "", err
	}
	if len(out) > max {
		cut := strings.LastIndex(out[:max], "\n")
		if cut < 0 {
			cut = max
		}
		out = out[:cut] + fmt.Sprintf("\n…（diff 太长，后面 %d KB 没有附上）\n", (len(out)-cut)/1024)
	}
	return out, nil
}

// Main is the root of the repository dir is in, and of the repository a
// linked worktree belongs to: agents that each work in a worktree of
// their own work on one project.
func Main(dir string) (string, error) {
	if !IsRepo(dir) {
		return "", fmt.Errorf("%s 不是 git 仓库", dir)
	}
	if common, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil && filepath.Base(common) == ".git" {
		return filepath.Dir(common), nil
	}
	return Root(dir)
}

// State says where the code in dir is, in a few words that change when a
// commit is made or a file is changed: the commit checked out, and what
// is not committed.
func State(dir string) (commit, dirty string, err error) {
	h, err := HeadOf(dir)
	if err != nil {
		return "", "", err
	}
	dirty, err = git(dir, "status", "--porcelain")
	return h.Commit, dirty, err
}

// Remote names the repository dir is a clone of, by where its origin is,
// as "github.com/owner/repo": the same on every computer that has it,
// which its folder is not. "" when it has no origin.
func Remote(dir string) string {
	if !IsRepo(dir) {
		return ""
	}
	url, err := git(dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return NormalRemote(url)
}

// NormalRemote is a remote's URL without what differs between ways of
// reaching one repository: the scheme, the user, the port, ".git".
func NormalRemote(url string) string {
	url = strings.TrimSpace(url)
	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
	} else if i := strings.Index(url, ":"); i > 0 && !strings.Contains(url[:i], "/") { // git@host:owner/repo
		url = url[:i] + "/" + url[i+1:]
	}
	if i := strings.LastIndex(url, "@"); i >= 0 {
		url = url[i+1:]
	}
	host, path, ok := strings.Cut(url, "/")
	if !ok || path == "" || host == "" || strings.HasPrefix(host, ".") { // a folder, not a host
		return ""
	}
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	return strings.ToLower(host + "/" + path)
}
