package proc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"astack/internal/workspace"
)

const ExitVerify = 5

const verifyTailBytes = 4000

// Verifier runs the --verify shell command in a workdir and reports pass/fail
// plus the output tail that is fed back to the worker on a retry.
type Verifier interface {
	Verify(command, dir string, timeout time.Duration, log io.Writer) VerifyResult
}

type VerifyResult struct {
	Code int
	Tail string
}

func (v VerifyResult) Passed() bool { return v.Code == 0 }

// ShellVerifier runs the command through cmd.exe on Windows and sh elsewhere.
type ShellVerifier struct {
	GOOS string
}

func NewShellVerifier() ShellVerifier {
	return ShellVerifier{GOOS: runtime.GOOS}
}

func (s ShellVerifier) Verify(command, dir string, timeout time.Duration, log io.Writer) VerifyResult {
	var cmd *exec.Cmd
	if s.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var buf bytes.Buffer
	w := io.MultiWriter(&buf, log)
	cmd.Stdout, cmd.Stderr = w, w
	code := runWithTimeout(cmd, timeout, log)
	return VerifyResult{Code: code, Tail: tail(buf.String(), verifyTailBytes)}
}

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration, log io.Writer) int {
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(log, err)
		return 1
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if timeout <= 0 {
		return exitFromErr(<-done, log)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return exitFromErr(err, log)
	case <-timer.C:
		killTree(cmd)
		<-done
		fmt.Fprintf(log, "astack: verify timed out after %s\n", timeout)
		return workspace.ExitTimeout
	}
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// retryTask is the next attempt's prompt: the original task plus why it failed.
func RetryTask(task, command string, res VerifyResult, attempt int) string {
	return task + fmt.Sprintf("\n\nAttempt %d failed verification. `%s` exited %d. Output tail:\n```\n%s\n```\n"+
		"Fix the code so that command passes. Do not weaken or delete the checks.", attempt, command, res.Code, res.Tail)
}
