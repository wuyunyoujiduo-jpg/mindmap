package automate

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed sdk/mymind/__init__.py
var sdk string

// SDK is the mymind module the programs import.
func SDK() string { return sdk }

// Statuses of a run.
const (
	Running = "running"
	OK      = "ok"
	Failed  = "error"
	Stopped = "stopped"
	TimeOut = "timeout"
)

// Run is one run of a program.
type Run struct {
	ID         string    `json:"id"` // its time, which names its file
	Automation string    `json:"automation"`
	By         string    `json:"by"` // 手动, 定时
	Start      time.Time `json:"start"`
	End        time.Time `json:"end,omitzero"`
	Status     string    `json:"status"`
	Exit       int       `json:"exit,omitempty"`
	Log        string    `json:"log,omitempty"`    // what it printed
	Output     string    `json:"output,omitempty"` // what it gave as its result
	Error      string    `json:"error,omitempty"`  // why it could not start or end
}

// maxLog is how much of what a run prints is kept: the last of it.
const maxLog = 256 << 10

// keepRuns is how many runs of each program are kept.
const keepRuns = 30

func runsDir(root, id string) string { return filepath.Join(root, ".runs", id) }

// Runs are the runs of a program kept, the last first.
func Runs(root, id string) []Run {
	entries, _ := os.ReadDir(runsDir(root, id))
	var out []Run
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(runsDir(root, id), e.Name()))
		if err != nil {
			continue
		}
		var r Run
		if json.Unmarshal(data, &r) == nil {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.After(out[j].Start) })
	return out
}

func saveRun(root string, r Run) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(runsDir(root, r.Automation), r.ID+".json"), data); err != nil {
		return err
	}
	entries, _ := os.ReadDir(runsDir(root, r.Automation))
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keepRuns {
		_ = os.Remove(filepath.Join(runsDir(root, r.Automation), names[0]))
		names = names[1:]
	}
	return nil
}

// Handler answers what a program asks of the app through mymind: the
// method's name and its parameters, as JSON. What it writes to out goes
// into the run's log, as the program's printing does.
type Handler func(ctx context.Context, a *Automation, out io.Writer, method string, params json.RawMessage) (any, error)

// Env is what runs need.
type Env struct {
	Root    string
	Python  string // "" finds one
	Handler Handler
	// Changed is told when a run printed more or ended.
	Changed func()
}

// Process is a run going on.
type Process struct {
	mu     sync.Mutex
	run    Run
	cancel context.CancelFunc
	stop   bool
	done   chan struct{}
}

// Snapshot is the run as it is now.
func (p *Process) Snapshot() Run {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.run
}

// Stop ends the run.
func (p *Process) Stop() {
	p.mu.Lock()
	p.stop = true
	p.mu.Unlock()
	p.cancel()
}

// Done is closed when the run has ended and was kept.
func (p *Process) Done() <-chan struct{} { return p.done }

func (p *Process) Write(b []byte) (int, error) {
	p.mu.Lock()
	p.run.Log += string(b)
	if len(p.run.Log) > maxLog {
		p.run.Log = "…\n" + p.run.Log[len(p.run.Log)-maxLog:]
	}
	p.mu.Unlock()
	return len(b), nil
}

// Start runs a program, by hand or by its schedule ("手动", "定时").
func Start(env Env, a *Automation, by string, now time.Time) (*Process, error) {
	if a.Err != "" {
		return nil, errors.New(a.Err)
	}
	python := env.Python
	if python == "" {
		python = FindPython(env.Root)
	}
	if python == "" {
		return nil, errors.New("找不到 Python 3：请先安装（brew install python 或 python.org）")
	}
	if err := writeSDK(env.Root); err != nil {
		return nil, err
	}
	sockDir, err := os.MkdirTemp("", "mm")
	if err != nil {
		return nil, err
	}
	sock := filepath.Join(sockDir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		os.RemoveAll(sockDir)
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.Timeout)
	p := &Process{cancel: cancel, done: make(chan struct{}), run: Run{
		ID: now.Format("20060102-150405.000"), Automation: a.ID, By: by, Start: now, Status: Running,
	}}
	changed := func() {
		if env.Changed != nil {
			env.Changed()
		}
	}
	w := &notifying{p: p, changed: changed}
	go p.serve(ctx, ln, env, a, w, changed)

	cmd := exec.CommandContext(ctx, python, "-u", a.EntryPath())
	cmd.Dir = a.Dir()
	cmd.Env = runEnv(env.Root, a, sock)
	cmd.Stdout, cmd.Stderr = w, w
	setGroup(cmd)
	if err := cmd.Start(); err != nil {
		cancel()
		ln.Close()
		os.RemoveAll(sockDir)
		return nil, err
	}
	began := time.Now() // the run's time goes by the clock of now
	go func() {
		err := cmd.Wait()
		ln.Close()
		os.RemoveAll(sockDir)
		p.mu.Lock()
		p.run.End = p.run.Start.Add(time.Since(began))
		switch {
		case p.stop:
			p.run.Status = Stopped
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			p.run.Status = TimeOut
			p.run.Error = fmt.Sprintf("超过 %s 没有结束，已停止", a.Timeout)
		case err != nil:
			p.run.Status = Failed
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				p.run.Exit = ee.ExitCode()
			} else {
				p.run.Error = err.Error()
			}
		default:
			p.run.Status = OK
		}
		r := p.run
		p.mu.Unlock()
		cancel()
		_ = saveRun(env.Root, r)
		close(p.done)
		changed()
	}()
	return p, nil
}

type notifying struct {
	p       *Process
	changed func()
}

func (w *notifying) Write(b []byte) (int, error) {
	n, err := w.p.Write(b)
	w.changed()
	return n, err
}

// serve answers the program's calls: a line of JSON each, answered by one.
func (p *Process) serve(ctx context.Context, ln net.Listener, env Env, a *Automation, out io.Writer, changed func()) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			r := bufio.NewReader(conn)
			for {
				line, err := r.ReadBytes('\n')
				if len(line) == 0 && err != nil {
					return
				}
				var req struct {
					ID     int             `json:"id"`
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				var resp struct {
					ID     int    `json:"id"`
					Result any    `json:"result"`
					Error  string `json:"error,omitempty"`
				}
				if e := json.Unmarshal(line, &req); e != nil {
					resp.Error = "看不懂的请求：" + e.Error()
				} else {
					resp.ID = req.ID
					var res any
					var e error
					if req.Method == "output" {
						res, e = p.output(req.Params)
						changed()
					} else if env.Handler != nil {
						res, e = env.Handler(ctx, a, out, req.Method, req.Params)
					} else {
						e = fmt.Errorf("不支持 %s", req.Method)
					}
					resp.Result = res
					if e != nil {
						resp.Error = e.Error()
					}
				}
				out, _ := json.Marshal(resp)
				if _, err := conn.Write(append(out, '\n')); err != nil {
					return
				}
				if err == io.EOF {
					return
				}
			}
		}()
	}
}

func (p *Process) output(params json.RawMessage) (any, error) {
	var in struct {
		Text   string `json:"text"`
		Append bool   `json:"append"`
	}
	if err := json.Unmarshal(params, &in); err != nil {
		return nil, err
	}
	p.mu.Lock()
	if in.Append && p.run.Output != "" {
		p.run.Output += "\n" + in.Text
	} else {
		p.run.Output = in.Text
	}
	p.mu.Unlock()
	return true, nil
}

// StatePath is the file of what a program keeps between runs.
func StatePath(root, id string) string {
	return filepath.Join(root, ".state", strings.TrimSuffix(id, ".py")+".json")
}

func runEnv(root string, a *Automation, sock string) []string {
	env := os.Environ()
	set := func(k, v string) {
		for i, e := range env {
			if strings.HasPrefix(e, k+"=") {
				env[i] = k + "=" + v
				return
			}
		}
		env = append(env, k+"="+v)
	}
	path := []string{filepath.Join(root, ".sdk"), filepath.Join(root, Shared)}
	if a.Folder {
		path = append(path, a.Path)
	}
	if old := os.Getenv("PYTHONPATH"); old != "" {
		path = append(path, old)
	}
	set("PYTHONPATH", strings.Join(path, string(os.PathListSeparator)))
	set("PATH", SearchPath())
	set("PYTHONUNBUFFERED", "1")
	set("PYTHONIOENCODING", "utf-8")
	set("PYTHONDONTWRITEBYTECODE", "1")
	set("MYMIND_SOCKET", sock)
	set("MYMIND_ROOT", root)
	set("MYMIND_AUTOMATION", a.ID)
	set("MYMIND_NAME", a.Name)
	set("MYMIND_STATE", StatePath(root, a.ID))
	set("MYMIND_MODEL", a.Model)
	set("MYMIND_AGENT", a.Agent)
	settings, _ := json.Marshal(a.Settings)
	set("MYMIND_SETTINGS", string(settings))
	return env
}

// SearchPath is PATH with the places programs are installed on a Mac,
// which an app opened from the Finder does not have.
func SearchPath() string {
	home, _ := os.UserHomeDir()
	dirs := filepath.SplitList(os.Getenv("PATH"))
	for _, d := range []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".local/bin"),
		filepath.Join(home, ".npm-global/bin"), filepath.Join(home, ".claude/local"), "/usr/bin", "/bin"} {
		if !containsString(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// VenvPython is the Python of the programs' shared environment.
func VenvPython(root string) string { return filepath.Join(root, ".venv", "bin", "python") }

// FindPython is the shared environment's Python, else the system's.
func FindPython(root string) string {
	if _, err := os.Stat(VenvPython(root)); err == nil {
		return VenvPython(root)
	}
	return systemPython()
}

func systemPython() string {
	for _, p := range []string{"/opt/homebrew/bin/python3", "/usr/local/bin/python3"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return p
	}
	if _, err := os.Stat("/usr/bin/python3"); err == nil {
		return "/usr/bin/python3"
	}
	return ""
}

func writeSDK(root string) error {
	path := filepath.Join(root, ".sdk", "mymind", "__init__.py")
	if old, err := os.ReadFile(path); err == nil && string(old) == sdk {
		return nil
	}
	return writeFile(path, []byte(sdk))
}

// Install makes the shared environment when it is missing, and installs
// what every program's requirements.txt asks, telling how it goes.
func Install(ctx context.Context, root string, log io.Writer) error {
	if _, err := os.Stat(VenvPython(root)); err != nil {
		py := systemPython()
		if py == "" {
			return errors.New("找不到 Python 3：请先安装（brew install python 或 python.org）")
		}
		fmt.Fprintf(log, "$ %s -m venv .venv\n", py)
		cmd := exec.CommandContext(ctx, py, "-m", "venv", filepath.Join(root, ".venv"))
		cmd.Stdout, cmd.Stderr = log, log
		cmd.Env = append(os.Environ(), "PATH="+SearchPath())
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("建环境失败：%w", err)
		}
	}
	var reqs []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "__pycache__") {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "requirements.txt" {
			reqs = append(reqs, path)
		}
		return nil
	})
	if len(reqs) == 0 {
		fmt.Fprintln(log, "没有 requirements.txt，不用安装。")
		return nil
	}
	args := []string{"-m", "pip", "install", "--disable-pip-version-check"}
	for _, r := range reqs {
		args = append(args, "-r", r)
	}
	fmt.Fprintf(log, "$ python %s\n", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, VenvPython(root), args...)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = log, log
	cmd.Env = append(os.Environ(), "PATH="+SearchPath())
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("安装失败：%w", err)
	}
	// Playwright drives browsers of its own, which it downloads.
	for _, r := range reqs {
		if data, _ := os.ReadFile(r); strings.Contains(strings.ToLower(string(data)), "playwright") {
			fmt.Fprintln(log, "$ python -m playwright install chromium")
			cmd := exec.CommandContext(ctx, VenvPython(root), "-m", "playwright", "install", "chromium")
			cmd.Stdout, cmd.Stderr = log, log
			cmd.Env = append(os.Environ(), "PATH="+SearchPath())
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("安装浏览器失败：%w", err)
			}
			break
		}
	}
	fmt.Fprintln(log, "安装完成。")
	return nil
}
