package cli

import (
	"bytes"
	"context"
	"io"
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

func TestNormalInvocationRequiresConfiguredRunner(t *testing.T) {
	cmd := NewCommand(Dependencies{}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"sample.pdf", "--parts", "2"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "split runner is not configured") {
		t.Fatalf("Execute() error = %v, want explicit missing-runner error", err)
	}
}

func TestCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "constraint required", args: []string{"input.pdf"}, wantErr: "at least one of --parts or --max-size is required"},
		{name: "parts zero", args: []string{"input.pdf", "--parts", "0"}, wantErr: "--parts must be positive"},
		{name: "parts negative", args: []string{"input.pdf", "--parts", "-1"}, wantErr: "--parts must be positive"},
		{name: "invalid max size", args: []string{"input.pdf", "--max-size", "1TB"}, wantErr: "invalid size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewCommand(Dependencies{Run: func(context.Context, Options) error { return nil }}, io.Discard, io.Discard)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCommandAcceptsCombinedConstraints(t *testing.T) {
	var got Options
	cmd := NewCommand(Dependencies{Run: func(_ context.Context, opts Options) error {
		got = opts
		return nil
	}}, io.Discard, io.Discard)
	cmd.SetArgs([]string{
		"input.pdf",
		"--parts", "4",
		"--max-size", "10MB",
		"--output", "result",
		"--overwrite",
		"--no-progress",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := Options{
		Input:      "input.pdf",
		Parts:      4,
		MaxSize:    10 << 20,
		OutputDir:  "result",
		Overwrite:  true,
		NoProgress: true,
	}
	if got != want {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
}

func TestCommandDefaultsOutputToCurrentDirectory(t *testing.T) {
	var got Options
	cmd := NewCommand(Dependencies{Run: func(_ context.Context, opts Options) error {
		got = opts
		return nil
	}}, io.Discard, io.Discard)
	cmd.SetArgs([]string{"input.pdf", "--parts", "2"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.OutputDir != "." {
		t.Fatalf("OutputDir = %q, want current directory", got.OutputDir)
	}
}

func TestHelpDocumentsSizeUnitsAndOversizedSinglePageBehavior(t *testing.T) {
	var stdout bytes.Buffer
	cmd := NewCommand(Dependencies{}, &stdout, io.Discard)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"KB", "MB", "GB", "single page", "--no-progress"} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("help missing %q: %s", text, stdout.String())
		}
	}
}

func TestHelpDocumentsPartsSemantics(t *testing.T) {
	var stdout bytes.Buffer
	cmd := NewCommand(Dependencies{}, &stdout, io.Discard)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"exactly N files when used alone", "minimum N files with --max-size"} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("help missing %q: %s", text, stdout.String())
		}
	}
}

func TestHelpDocumentsCombinedBehaviorAndExamples(t *testing.T) {
	var stdout bytes.Buffer
	cmd := NewCommand(Dependencies{}, &stdout, io.Discard)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"When both constraints", "Examples:", "report.pdf --parts 4"} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("help missing %q: %s", text, stdout.String())
		}
	}
}
