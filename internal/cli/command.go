package cli

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
)

type Dependencies struct{}

func NewCommand(_ Dependencies, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pdf-split <input.pdf>",
		Short: "Split a PDF into ordered continuous page ranges",
		Args:  cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return errors.New("PDF splitting is not implemented")
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd
}
