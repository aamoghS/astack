package main

import (
	_ "embed"
	"os"

	"astack/internal/app"
)

//go:embed agents.json
var agentsJSON []byte

func main() {
	os.Exit(app.NewDispatcher(agentsJSON).Main(os.Args[1:]))
}
