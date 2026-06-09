package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Black-Mamba24/pdf-split/internal/app"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Run func(context.Context, app.Options) error
}

type Options = app.Options

func NewCommand(deps Dependencies, stdout, stderr io.Writer) *cobra.Command {
	var (
		parts       int
		maxSizeText string
		outputDir   string
		overwrite   bool
		noProgress  bool
	)

	cmd := &cobra.Command{
		Use:   "pdf-split <input.pdf>",
		Short: "Split a PDF into ordered continuous page ranges",
		Long: `Split a PDF into ordered continuous page ranges.

Sizes use case-insensitive binary KB, MB, or GB units. A single page larger
than --max-size is still emitted with a warning.

When both constraints are supplied, the output count is at least --parts and
every non-single-page output respects --max-size.

Examples:
  pdf-split report.pdf --parts 4
  pdf-split report.pdf --max-size 10MB --output ./result
  pdf-split report.pdf --parts 4 --max-size 10MB --overwrite`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return invalidArgument(err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			partsSet := cmd.Flags().Changed("parts")
			maxSizeSet := cmd.Flags().Changed("max-size")
			if !partsSet && !maxSizeSet {
				return invalidArgument(errors.New("at least one of --parts or --max-size is required"))
			}
			if partsSet && parts < 1 {
				return invalidArgument(errors.New("--parts must be positive"))
			}

			var maxSize int64
			if maxSizeSet {
				var err error
				maxSize, err = ParseSize(maxSizeText)
				if err != nil {
					return invalidArgument(fmt.Errorf("--max-size: %w", err))
				}
			}
			if deps.Run == nil {
				return errors.New("split runner is not configured")
			}

			return deps.Run(cmd.Context(), app.Options{
				Input:      args[0],
				Parts:      parts,
				MaxSize:    maxSize,
				OutputDir:  outputDir,
				Overwrite:  overwrite,
				NoProgress: noProgress,
			})
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().IntVar(&parts, "parts", 0, "exactly N size-balanced files alone; minimum N files with --max-size")
	cmd.Flags().StringVar(&maxSizeText, "max-size", "", "maximum output size using KB, MB, or GB")
	cmd.Flags().StringVarP(&outputDir, "output", "o", ".", "output directory")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace existing output files after successful generation")
	cmd.Flags().BoolVar(&noProgress, "no-progress", false, "disable progress display")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return invalidArgument(err)
	})
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd
}

func invalidArgument(err error) error {
	return &app.ExitError{Code: 2, Err: err}
}
