package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type Dependencies struct {
	Run func(context.Context, Options) error
}

type Options struct {
	Input      string
	Parts      int
	MaxSize    int64
	OutputDir  string
	Overwrite  bool
	NoProgress bool
}

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
than --max-size is still emitted with a warning.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			partsSet := cmd.Flags().Changed("parts")
			maxSizeSet := cmd.Flags().Changed("max-size")
			if !partsSet && !maxSizeSet {
				return errors.New("at least one of --parts or --max-size is required")
			}
			if partsSet && parts < 1 {
				return errors.New("--parts must be positive")
			}

			var maxSize int64
			if maxSizeSet {
				var err error
				maxSize, err = ParseSize(maxSizeText)
				if err != nil {
					return fmt.Errorf("--max-size: %w", err)
				}
			}
			if deps.Run == nil {
				return errors.New("split runner is not configured")
			}

			return deps.Run(cmd.Context(), Options{
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
	cmd.Flags().IntVar(&parts, "parts", 0, "minimum number of output files")
	cmd.Flags().StringVar(&maxSizeText, "max-size", "", "maximum output size using KB, MB, or GB")
	cmd.Flags().StringVarP(&outputDir, "output", "o", ".", "output directory")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace existing output files after successful generation")
	cmd.Flags().BoolVar(&noProgress, "no-progress", false, "disable progress display")
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd
}
