// Package runengine tests flk-run, the run engine that core/run-wrapper
// copies into apps (SPEC 6.6), against testdata/fakeserver. The tests run
// bash on the host, so they cover the engine's logic, not the VM: PLAN
// M1's deploy test covers that.
package runengine

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var (
	engine, _ = filepath.Abs("../../snippets/core/run-wrapper/files/flk-run")
	fake, _   = filepath.Abs("../../testdata/fakeserver/fakeserver")
)

// run is one flk-run process and its output.
type run struct {
	t     *testing.T
	cmd   *exec.Cmd
	data  string
	mu    sync.Mutex
	lines []string
	done  chan struct{}
	code  int
}

// start runs flk-run -- fakeserver with env on top of fast defaults.
func start(t *testing.T, env map[string]string, args ...string) *run {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	t.Parallel()
	r := &run{t: t, data: t.TempDir(), done: make(chan struct{})}
	hooks := t.TempDir()
	base := map[string]string{
		"RUN_NAME": "test", "RUN_HOOKS": hooks, "FAKE_DATA": r.data,
		"FAKE_READY_AFTER": "0.2", "RESTART_DELAY": "0", "STOP_TIMEOUT": "5",
		"RUN_READY": "Server ready", "PATH": os.Getenv("PATH"),
	}
	for k, v := range env {
		base[k] = v
	}
	if len(args) == 0 {
		args = []string{"--", fake}
	}
	r.cmd = exec.Command(engine, args...)
	for k, v := range base {
		r.cmd.Env = append(r.cmd.Env, k+"="+v)
	}
	r.cmd.Stdin = nil // /dev/null, as with [flk] stdin = "null"
	out, err := r.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	r.cmd.Stderr = r.cmd.Stdout
	if err := r.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			r.mu.Lock()
			r.lines = append(r.lines, sc.Text())
			r.mu.Unlock()
		}
		r.cmd.Wait()
		r.code = r.cmd.ProcessState.ExitCode()
		close(r.done)
	}()
	t.Cleanup(func() {
		r.cmd.Process.Kill()
		<-r.done
	})
	return r
}

// hook writes a hook file.
func hook(t *testing.T, env map[string]string, name, body string) {
	t.Helper()
	if env["RUN_HOOKS"] == "" {
		env["RUN_HOOKS"] = t.TempDir()
	}
	if err := os.WriteFile(filepath.Join(env["RUN_HOOKS"], name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (r *run) output() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

// wait waits for a line matching re.
func (r *run) wait(re string) {
	r.t.Helper()
	rx := regexp.MustCompile(re)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if rx.MatchString(r.output()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.t.Fatalf("no line matching %q in:\n%s", re, r.output())
}

// exit waits for flk-run to exit and checks its code.
func (r *run) exit(code int) {
	r.t.Helper()
	select {
	case <-r.done:
	case <-time.After(15 * time.Second):
		r.t.Fatalf("flk-run didn't exit:\n%s", r.output())
	}
	if r.code != code {
		r.t.Fatalf("exit %d, want %d:\n%s", r.code, code, r.output())
	}
}

func (r *run) term() { r.cmd.Process.Signal(syscall.SIGTERM) }

func (r *run) saved() bool {
	_, err := os.Stat(filepath.Join(r.data, "world.txt"))
	return err == nil
}

func TestConsoleStop(t *testing.T) {
	r := start(t, map[string]string{"RUN_STOP": "console:stop", "FAKE_STOP": "console"})
	r.wait(`"type":"server.online"`)
	r.term()
	r.wait(`fake: stopping \(console\)`)
	r.exit(0)
	for _, want := range []string{`"type":"server.starting"`, `"type":"server.stopping"`, `"type":"server.stopped"`, "fake: Saved the world"} {
		if !strings.Contains(r.output(), want) {
			t.Errorf("missing %s", want)
		}
	}
	if !r.saved() {
		t.Error("no world.txt: the stop didn't save")
	}
}

func TestSignalINTStop(t *testing.T) {
	r := start(t, map[string]string{"RUN_STOP": "signal:INT", "FAKE_STOP": "INT"})
	r.wait("Server ready")
	r.term()
	r.wait(`fake: stopping \(SIGINT\)`)
	r.exit(0)
	if !r.saved() {
		t.Error("no world.txt: SIGINT didn't reach the server (ignored in the background?)")
	}
}

func TestEscalation(t *testing.T) {
	r := start(t, map[string]string{"RUN_STOP": "console:stop", "FAKE_HANG": "1", "STOP_TIMEOUT": "1",
		"RUN_ESCALATE": "KILL", "RUN_ESCALATE_CMD": "echo cleanup ran"})
	r.wait("Server ready")
	r.term()
	r.wait("ignoring the stop")
	r.wait(`didn't stop in 1s; sending SIGKILL`)
	r.wait("cleanup ran")
	r.exit(0)
	if r.saved() {
		t.Error("a killed server shouldn't have saved")
	}
}

func TestEscalationDefaultTerm(t *testing.T) {
	r := start(t, map[string]string{"RUN_STOP": "console:stop", "FAKE_HANG": "1", "STOP_TIMEOUT": "1"})
	r.wait("Server ready")
	r.term()
	r.wait("sending SIGTERM")
	r.wait("killed by SIGTERM")
	r.exit(0)
}

func TestRestartsThenGivesUp(t *testing.T) {
	r := start(t, map[string]string{"FAKE_CRASH_AFTER": "1", "MAX_RESTARTS": "2", "RESTART_WINDOW": "600"})
	r.wait(`"type":"server.gave_up"`)
	r.exit(1)
	out := r.output()
	if n := strings.Count(out, `"type":"server.restarting"`); n != 2 {
		t.Errorf("%d restarts, want 2", n)
	}
	if n := strings.Count(out, `"type":"server.online"`); n != 3 {
		t.Errorf("%d online events, want one per start (3)", n)
	}
	if b, _ := os.ReadFile(filepath.Join(r.data, "starts")); strings.TrimSpace(string(b)) != "3" {
		t.Errorf("starts = %q, want 3", b)
	}
}

func TestStopDuringRestartDelay(t *testing.T) {
	r := start(t, map[string]string{"FAKE_CRASH_AFTER": "1", "RESTART_DELAY": "30"})
	r.wait(`restarting in 30s`)
	began := time.Now()
	r.term()
	r.wait("not restarting")
	r.exit(0)
	if time.Since(began) > 5*time.Second {
		t.Error("the stop waited for the restart delay")
	}
}

func TestFilterAndReady(t *testing.T) {
	r := start(t, map[string]string{"RUN_FILTER": "^fake: starting", "RUN_READY": "Server ready"})
	r.wait(`"type":"server.online"`)
	r.term()
	r.exit(0)
	if strings.Contains(r.output(), "fake: starting") {
		t.Error("RUN_FILTER didn't drop the line")
	}
}

func TestEventsOff(t *testing.T) {
	r := start(t, map[string]string{"RUN_EVENTS": "0"})
	r.wait("Server ready")
	r.term()
	r.exit(0)
	if strings.Contains(r.output(), "[gamebus]") {
		t.Error("events printed with RUN_EVENTS=0")
	}
}

func TestHooks(t *testing.T) {
	env := map[string]string{"RUN_STOP": "console:stop", "RUN_TICK": "1"}
	hook(t, env, "prestart", "#!/bin/sh\necho prestart ran; echo x >> \"$FAKE_DATA/prestarts\"\n")
	hook(t, env, "watch", `
on_line() {
  [[ $1 == *"Server ready"* ]] && event test.ready "saw it" n:=1
  return 0
}
on_tick() { console "say tick"; }
`)
	hook(t, env, "poststop", "#!/bin/sh\necho \"poststop status=$1 stopping=$RUN_STOPPING\"\n")
	r := start(t, env)
	r.wait(`"type":"test.ready","msg":"saw it","n":1`)
	r.wait("fake: tick") // on_tick typed into the console
	r.term()
	r.wait("poststop status=0 stopping=1")
	r.exit(0)
	if !strings.Contains(r.output(), "prestart ran") {
		t.Error("prestart didn't run")
	}
}

func TestPrestartRunsBeforeEveryStart(t *testing.T) {
	env := map[string]string{"FAKE_CRASH_AFTER": "1", "MAX_RESTARTS": "1"}
	hook(t, env, "prestart", "#!/bin/sh\necho x >> \"$FAKE_DATA/prestarts\"\n")
	hook(t, env, "poststop", "#!/bin/sh\necho \"poststop status=$1 stopping=$RUN_STOPPING\"\n")
	r := start(t, env)
	r.exit(1)
	b, _ := os.ReadFile(filepath.Join(r.data, "prestarts"))
	if strings.Count(string(b), "x") != 2 {
		t.Errorf("prestart ran %d times, want 2", strings.Count(string(b), "x"))
	}
	if !strings.Contains(r.output(), "poststop status=3 stopping=0") {
		t.Error("poststop didn't get the crash status")
	}
}

func TestPrestartFailureStops(t *testing.T) {
	env := map[string]string{}
	hook(t, env, "prestart", "#!/bin/sh\necho 'FATAL: no config'; exit 2\n")
	r := start(t, env)
	r.exit(1)
	if strings.Contains(r.output(), "fake: starting") {
		t.Error("the server started after prestart failed")
	}
}

func TestExecMode(t *testing.T) {
	env := map[string]string{}
	hook(t, env, "prestart", "#!/bin/sh\necho prestart ran\n")
	r := start(t, env, "--exec", "--", "sh", "-c", "echo pid=$$; exit 7")
	r.exit(7)
	out := r.output()
	if !strings.Contains(out, "prestart ran") || strings.Contains(out, "[gamebus]") {
		t.Errorf("--exec: %s", out)
	}
	if !strings.Contains(out, "pid="+strings.TrimSpace(itoa(r.cmd.Process.Pid))) {
		t.Errorf("--exec didn't exec (pid differs): %s", out)
	}
}

func TestBadSettings(t *testing.T) {
	for _, env := range []map[string]string{{"RUN_STOP": "bogus"}, {"STOP_TIMEOUT": "soon"}} {
		t.Run(strings.Join(keys(env), ","), func(t *testing.T) {
			r := start(t, env)
			r.exit(1)
			if !strings.Contains(r.output(), "FATAL") {
				t.Errorf("%v: %s", env, r.output())
			}
		})
	}
}

func itoa(n int) string {
	var b [20]byte
	i := len(b)
	for n > 0 || i == len(b) {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestStopDuringPrestart(t *testing.T) {
	env := map[string]string{}
	hook(t, env, "prestart", "#!/bin/sh\necho prestart sleeping; sleep 1\n")
	r := start(t, env)
	r.wait("prestart sleeping")
	r.term()
	r.wait("stop requested before the server started")
	r.exit(0)
	if strings.Contains(r.output(), "fake: starting") {
		t.Error("the server started after the stop")
	}
}
