package main

import (
	"fmt"
	"os"

	"github.com/Black-Mamba24/pdf-split/internal/cli"
)

func main() {
	if err := cli.NewCommand(cli.Dependencies{}, os.Stdout, os.Stderr).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(8)
	}
}
