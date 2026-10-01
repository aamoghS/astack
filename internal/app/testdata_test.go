package app

import (
	"os"
	"path/filepath"
)

// embeddedAgents is the repo's agents.json, found by walking up from the test's directory.
var embeddedAgents = func() []byte {
	dir, _ := os.Getwd()
	for {
		if b, err := os.ReadFile(filepath.Join(dir, "agents.json")); err == nil {
			return b
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("agents.json not found")
		}
		dir = parent
	}
}()
