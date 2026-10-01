package proc

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"astack/internal/platform"
	"astack/internal/workspace"
)

// Runner executes a worker process.
type Runner interface {
	Run(bin string, args []string, cwd string) int
}

// OutputRunner is a Runner that can hand a worker's stdout to the caller
// (panels read the reply instead of streaming it). Stdin is closed.
type OutputRunner interface {
	RunOutput(bin string, args []string, cwd string, stdout, stderr io.Writer) int
}

// ProcessRunner builds an *exec.Cmd through the Platform (cmd.exe shims on Windows).
type ProcessRunner struct {
	Platform platform.Platform
	Environ  []string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Timeout  time.Duration
}

func NewProcessRunner(p platform.Platform) *ProcessRunner {
	return &ProcessRunner{
		Platform: p,
		Environ:  os.Environ(),
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}
}

func (r *ProcessRunner) Command(bin string, args []string, cwd string) *exec.Cmd {
	exe, argv := r.Platform.Wrap(bin, args)
	cmd := exec.Command(exe, argv...)
	cmd.Dir = cwd
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	cmd.Env = r.Platform.AugmentEnv(r.Environ)
	return cmd
}

func (r *ProcessRunner) Run(bin string, args []string, cwd string) int {
	cmd := r.Command(bin, args, cwd)
	if r.Timeout <= 0 {
		return waitCmd(cmd, r.Stderr)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(r.Stderr, err)
		return 1
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(r.Timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return exitFromErr(err, r.Stderr)
	case <-timer.C:
		killTree(cmd)
		<-done
		fmt.Fprintf(r.Stderr, "astack: worker timed out after %s\n", r.Timeout)
		return workspace.ExitTimeout
	}
}

func (r *ProcessRunner) RunOutput(bin string, args []string, cwd string, stdout, stderr io.Writer) int {
	cp := *r
	cp.Stdin = strings.NewReader("")
	cp.Stdout, cp.Stderr = stdout, stderr
	return cp.Run(bin, args, cwd)
}

func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		return
	}
	_ = cmd.Process.Kill()
}

func waitCmd(cmd *exec.Cmd, stderr io.Writer) int {
	return exitFromErr(cmd.Run(), stderr)
}

func exitFromErr(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	fmt.Fprintln(stderr, err)
	return 1
}

// PromptStore writes the task file inside the workdir so Codex's sandbox can read it.
// Tag keeps parallel panel members in one workdir from sharing a file.
type PromptStore struct {
	Pid int
	Tag string
}

func NewPromptStore() PromptStore {
	return PromptStore{Pid: os.Getpid()}
}

func (s PromptStore) Write(workdir, body string) (string, error) {
	name := ".astack-prompt." + strconv.Itoa(s.Pid)
	if s.Tag != "" {
		name += "." + s.Tag
	}
	name += ".txt"
	path := filepath.Join(workdir, name)
	return path, os.WriteFile(path, []byte(body), 0600)
}

func (PromptStore) Remove(path string) {
	_ = os.Remove(path)
}
