package workspace

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	changeCapBytes  = 60000
	newFileCapBytes = 8000
)

func AbsDir(dir string) (string, error) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("workdir not found: %s", dir)
	}
	return filepath.Abs(dir)
}

// GitChange is the uncommitted change in a git workdir: tracked diff against
// HEAD plus untracked files. Empty when clean or not a repo.
func GitChange(dir string) string {
	var b strings.Builder
	diff, err := gitOut(dir, "diff", "HEAD", "--no-color")
	if err != nil {
		diff, _ = gitOut(dir, "diff", "--no-color")
	}
	b.WriteString(strings.TrimRight(diff, "\n"))
	untracked, _ := gitOut(dir, "ls-files", "--others", "--exclude-standard")
	for _, rel := range strings.Split(strings.TrimSpace(untracked), "\n") {
		rel = strings.TrimSpace(rel)
		if rel == "" || SkipName(filepath.Base(rel)) {
			continue
		}
		writeNewFile(&b, rel, filepath.Join(dir, rel))
	}
	return capText(strings.TrimSpace(b.String()), changeCapBytes)
}

func gitOut(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return string(out), err
}

// TreeChange diffs an isolated copy against the workdir it was copied from,
// using the same skip rules as the copy. Arena arms have no .git, so this is
// how the judge sees what each arm did.
func TreeChange(orig, copy string) string {
	before := treeFiles(orig)
	after := treeFiles(copy)
	paths := map[string]bool{}
	for p := range before {
		paths[p] = true
	}
	for p := range after {
		paths[p] = true
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	var b strings.Builder
	for _, rel := range sorted {
		a, inA := before[rel]
		c, inC := after[rel]
		switch {
		case !inA:
			writeNewFile(&b, rel, c)
		case !inC:
			fmt.Fprintf(&b, "\n--- deleted %s\n", rel)
		default:
			if sameFile(a, c) {
				continue
			}
			out, _ := exec.Command("git", "diff", "--no-index", "--no-color", "--", a, c).Output()
			if len(out) == 0 {
				fmt.Fprintf(&b, "\n--- modified %s (diff unavailable)\n", rel)
				continue
			}
			fmt.Fprintf(&b, "\n%s", strings.TrimRight(string(out), "\n"))
		}
	}
	return capText(strings.TrimSpace(b.String()), changeCapBytes)
}

func treeFiles(root string) map[string]string {
	files := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if SkipName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			rel, err := filepath.Rel(root, path)
			if err == nil {
				files[filepath.ToSlash(rel)] = path
			}
		}
		return nil
	})
	return files
}

func sameFile(a, b string) bool {
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

func writeNewFile(b *strings.Builder, rel, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	fmt.Fprintf(b, "\n+++ new file %s\n", rel)
	if bytes.IndexByte(data, 0) >= 0 {
		b.WriteString("(binary)\n")
		return
	}
	b.WriteString(capText(string(data), newFileCapBytes))
	b.WriteString("\n")
}

func capText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n... (truncated, %d more bytes)", len(s)-n)
}
