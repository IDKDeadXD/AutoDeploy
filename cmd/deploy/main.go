package main

import (
	"fmt"
	"os"

	"github.com/idkde/deploy-agent/internal/cli"
)

func main() {
	if err := cli.New().Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "deploy: error:", err)
		os.Exit(1)
	}
}
