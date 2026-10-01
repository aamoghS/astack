package workspace

import (
	"os/exec"
	"strings"
)

func DiffStat(workdir string) string {
	if workdir == "" {
		return ""
	}
	cmd := exec.Command("git", "-C", workdir, "diff", "--stat")
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
