package main

import (
	_ "embed"
	"os"
)

//go:embed agents.json
var agentsJSON []byte

func main() {
	os.Exit(NewDispatcher(agentsJSON).Main(os.Args[1:]))
}
