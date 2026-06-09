package cli

import (
	"bytes"
	"testing"
)

func TestHelpSucceedsWithoutInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewCommand(Dependencies{}, &stdout, &stderr)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("pdf-split <input.pdf>")) {
		t.Fatalf("help missing usage: %q", stdout.String())
	}
}
