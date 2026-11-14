package mcp

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/gitbase"
)

// Hooks: what makes the record of a project not depend on an agent
// remembering to keep it. Claude Code runs "MyMind hook start" as a
// session begins and puts what it prints before the agent, the project's
// brief; and "MyMind hook stop" as the agent is about to stop, which,
// when the code changed since the project's record was last written to,
// has the agent call log_session first. It asks for a commit made, and
// for changes not committed no more than every askEvery: a session
// of many turns is not asked at each.

// askEvery is how long the stop hook waits before it asks again for
// changes that are not committed.
const askEvery = 30 * time.Minute

// codeState is where a project's code was when its record was last
// brought up to date, or the agent last asked to.
type codeState struct {
	Commit string    `json:"commit"`
	Dirty  string    `json:"dirty"` // a hash of what is not committed
	At     time.Time `json:"at"`
	// Asked is when the agent was last asked to record the session, or did.
	Asked time.Time `json:"asked,omitzero"`
}

// stateFile is where the state of the code in dir is kept: by the folder
// the agent works in, a worktree its own.
func (s *Server) stateFile(dir string) string {
	if s.StateDir == "" {
		return ""
	}
	if r, err := gitbase.Root(dir); err == nil {
		dir = r
	}
	h := sha1.Sum([]byte(filepath.Clean(dir)))
	return filepath.Join(s.StateDir, hex.EncodeToString(h[:8])+".json")
}

func (s *Server) stateOf(dir string) (codeState, bool) {
	commit, dirty, err := gitbase.State(dir)
	if err != nil {
		return codeState{}, false
	}
	h := sha1.Sum([]byte(dirty))
	return codeState{Commit: commit, Dirty: hex.EncodeToString(h[:8]), At: s.Now()}, true
}

func (s *Server) lastState(dir string) (codeState, bool) {
	var st codeState
	data, err := os.ReadFile(s.stateFile(dir))
	if err != nil || json.Unmarshal(data, &st) != nil {
		return st, false
	}
	return st, true
}

func (s *Server) keepState(dir string, st codeState) {
	path := s.stateFile(dir)
	if path == "" {
		return
	}
	if data, err := json.Marshal(st); err == nil && os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
}

// reported notes that the project's record is up to date with the code
// as it is.
func (s *Server) reported(dir string) {
	if dir == "" {
		dir = s.Cwd
	}
	if st, ok := s.stateOf(dir); ok {
		st.Asked = st.At
		s.keepState(dir, st)
	}
}

// HookStart is what an agent is shown as a session begins in dir: the
// project's brief, or nothing when the project has no map yet.
func (s *Server) HookStart(dir string) string {
	if st, ok := s.stateOf(dir); ok { // what changed before the session is not its to record
		s.keepState(dir, st)
	}
	path, f, err := s.projectMap(dir, false)
	if err != nil {
		return ""
	}
	return s.briefOf(path, f) + "\n（以上来自 MyMind。工作中：完成或开始任务用 update_node 改状态；新想到的待办用 capture；做了取舍用 log_decision；收工前用 log_session 记一两句。）\n"
}

// stopAsk is what the agent is told when it should record the session.
const stopAsk = "这次会话改动了代码，MyMind 里这个项目的记录还没跟上。结束前请调用 mymind 的 log_session：summary 用一两句话说清做了什么，没做完和新发现的事放进 todos；做过重要取舍的再各用一次 log_decision（决定了什么、为什么、没选什么）；完成了的任务用 update_node 标为 done。记完就可以结束，不用向用户复述。"

// HookStop answers Claude Code's Stop hook for a session in dir: nothing,
// to let the agent stop, or the decision that has it record the session
// first. again says the agent was already kept from stopping this turn.
func (s *Server) HookStop(dir string, again bool) string {
	if again {
		return ""
	}
	now, ok := s.stateOf(dir)
	if !ok {
		return ""
	}
	last, had := s.lastState(dir)
	if !had { // a session the start hook did not see: from here on
		s.keepState(dir, now)
		return ""
	}
	if now.Commit == last.Commit && (now.Dirty == last.Dirty || now.At.Sub(last.Asked) < askEvery) {
		return ""
	}
	now.Asked = now.At
	s.keepState(dir, now) // asked once for these changes, whatever the agent does
	out, _ := json.Marshal(map[string]string{"decision": "block", "reason": stopAsk})
	return string(out)
}
