package main

import (
	"os"
	"os/exec"
	"sync"
)

// FileLooker is the filesystem/PATH object BinaryResolver uses.
type FileLooker interface {
	LookPath(name string) string
	IsFile(path string) bool
}

type OSLooker struct{}

func (OSLooker) LookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

func (OSLooker) IsFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// BinaryResolver finds a worker executable on PATH, then platform extra dirs.
type BinaryResolver struct {
	Platform Platform
	Looker   FileLooker
}

func NewBinaryResolver(p Platform) BinaryResolver {
	return BinaryResolver{Platform: p, Looker: OSLooker{}}
}

func (r BinaryResolver) Resolve(a Agent) string {
	for _, b := range a.Bins {
		if hit := r.Looker.LookPath(b); hit != "" {
			return hit
		}
	}
	for _, p := range r.Platform.ExtraBins(a.Bins) {
		if r.Looker.IsFile(p) {
			return p
		}
	}
	return ""
}

func (r BinaryResolver) ResolveAll(reg *AgentRegistry) map[string]string {
	all := reg.All()
	out := make(map[string]string, len(all))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for id, a := range all {
		wg.Add(1)
		go func(id string, a Agent) {
			defer wg.Done()
			p := r.Resolve(a)
			mu.Lock()
			out[id] = p
			mu.Unlock()
		}(id, a)
	}
	wg.Wait()
	return out
}
