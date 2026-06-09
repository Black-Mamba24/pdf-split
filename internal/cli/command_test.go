package cli

import (
	"bytes"
	"strings"
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

func TestNormalInvocationReturnsNotImplementedError(t *testing.T) {
	cmd := NewCommand(Dependencies{}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"sample.pdf"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "PDF splitting is not implemented") {
		t.Fatalf("Execute() error = %v, want explicit not-implemented error", err)
	}
}
