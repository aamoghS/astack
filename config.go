package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type agentSpec struct {
	Bin       []string `json:"bin"`
	Args      []string `json:"args"`
	ReadArgs  []string `json:"read_args"`
	ModelArgs []string `json:"model_args"`
}

type configFile struct {
	Default string               `json:"default"`
	Order   []string             `json:"order"`
	Footer  string               `json:"footer"`
	Agents  map[string]agentSpec `json:"agents"`
	Roles   map[string]Role      `json:"roles"`
}

const defaultFooter = "You are the astack implementer. The current IDE or chat agent is the conductor. Follow AGENTS.md or CLAUDE.md if present. Edit files in this repo. Do not commit, push, or deploy unless the prompt says so."

// ConfigLoader finds agents.json (env, cwd, binary dir) and builds the registry.
type ConfigLoader struct {
	Embed []byte
	Env   func(string) string
	Cwd   func() (string, error)
	Exe   func() (string, error)
	Read  func(string) ([]byte, error)
}

func NewConfigLoader(embed []byte) *ConfigLoader {
	return &ConfigLoader{
		Embed: embed,
		Env:   os.Getenv,
		Cwd:   os.Getwd,
		Exe:   os.Executable,
		Read:  os.ReadFile,
	}
}

func (c *ConfigLoader) Load() (*AgentRegistry, string, error) {
	raw, src, err := c.bytes()
	if err != nil {
		return nil, src, err
	}
	reg, err := ParseRegistry(raw)
	return reg, src, err
}

func (c *ConfigLoader) bytes() ([]byte, string, error) {
	if c.Env != nil {
		if p := c.Env("ASTACK_AGENTS"); p != "" {
			b, err := c.Read(p)
			if err != nil {
				return nil, p, fmt.Errorf("ASTACK_AGENTS %s: %w", p, err)
			}
			return b, p, nil
		}
	}
	var paths []string
	if c.Cwd != nil {
		if cwd, err := c.Cwd(); err == nil {
			paths = append(paths, filepath.Join(cwd, "agents.json"))
		}
	}
	if c.Exe != nil {
		if exe, err := c.Exe(); err == nil && !isEphemeralExe(exe) {
			dir := filepath.Dir(exe)
			paths = append(paths, filepath.Join(dir, "agents.json"), filepath.Join(dir, "..", "agents.json"))
		}
	}
	for _, p := range paths {
		b, err := c.Read(p)
		if err == nil {
			return b, p, nil
		}
	}
	return c.Embed, "embed", nil
}

func isEphemeralExe(exe string) bool {
	s := strings.ToLower(filepath.ToSlash(exe))
	return strings.Contains(s, "/go-build") || strings.Contains(s, "/go-trybuild")
}

func ParseRegistry(b []byte) (*AgentRegistry, error) {
	var file configFile
	if err := json.Unmarshal(b, &file); err != nil {
		return nil, err
	}
	if file.Agents == nil {
		file.Agents = map[string]agentSpec{}
	}
	reg := &AgentRegistry{
		Default: file.Default,
		Order:   file.Order,
		Footer:  file.Footer,
		Roles:   file.Roles,
		byID:    map[string]Agent{},
	}
	if reg.Footer == "" {
		reg.Footer = defaultFooter
	}
	for id, spec := range file.Agents {
		if len(spec.Bin) == 0 {
			return nil, fmt.Errorf("agent %s has no bin", id)
		}
		if len(spec.Args) == 0 {
			return nil, fmt.Errorf("agent %s has no args", id)
		}
		reg.byID[id] = Agent{ID: id, Bins: spec.Bin, Args: spec.Args, ReadArgs: spec.ReadArgs, ModelArgs: spec.ModelArgs}
	}
	for _, id := range reg.Order {
		if _, ok := reg.byID[id]; !ok {
			return nil, fmt.Errorf("order lists unknown agent %s", id)
		}
	}
	for name, role := range reg.Roles {
		for _, id := range role.Agents {
			if _, ok := reg.byID[id]; !ok {
				return nil, fmt.Errorf("role %s lists unknown agent %s", name, id)
			}
		}
	}
	return reg, nil
}
