package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/Black-Mamba24/pdf-split/internal/app"
	"github.com/Black-Mamba24/pdf-split/internal/cli"
	"github.com/Black-Mamba24/pdf-split/internal/progress"
	"golang.org/x/term"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	deps := app.Dependencies{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		NewReporter: func(noProgress bool) progress.Reporter {
			terminal := term.IsTerminal(int(os.Stderr.Fd()))
			return progress.NewWithTerminal(os.Stderr, !noProgress, terminal)
		},
	}
	cmd := cli.NewCommand(cli.Dependencies{Run: func(ctx context.Context, opts app.Options) error {
		return app.Run(ctx, opts, deps)
	}}, os.Stdout, os.Stderr)
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(app.ExitCode(err))
	}
}
