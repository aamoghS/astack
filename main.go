package main

import (
	_ "embed"
	"os"
)

//go:embed agents.json
var embeddedAgents []byte

const defaultFooter = "You are the astack implementer. The current IDE or chat agent is the conductor. Follow AGENTS.md or CLAUDE.md if present. Edit files in this repo. Do not commit, push, or deploy unless the prompt says so."

func main() {
	os.Exit(NewDispatcher().Main(os.Args[1:]))
}
