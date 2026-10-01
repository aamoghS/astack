package proc

import (
	"bytes"
	"strings"
	"testing"
)

func TestShellVerifier(t *testing.T) {
	var log bytes.Buffer
	res := NewShellVerifier().Verify("echo hello-verify", t.TempDir(), 0, &log)
	if !res.Passed() || !strings.Contains(res.Tail, "hello-verify") {
		t.Fatal(res, log.String())
	}
	if res := NewShellVerifier().Verify("exit 3", t.TempDir(), 0, &log); res.Code != 3 {
		t.Fatal(res)
	}
}

func TestPromptStoreTag(t *testing.T) {
	p, err := PromptStore{Pid: 3, Tag: "why.claude"}.Write(t.TempDir(), "x")
	if err != nil || !strings.HasSuffix(p, ".astack-prompt.3.why.claude.txt") {
		t.Fatal(p, err)
	}
}
