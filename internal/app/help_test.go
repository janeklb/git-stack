package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestHelpWrapWidthForTerminalWidth(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{width: 80, want: 0},
		{width: 99, want: 0},
		{width: 100, want: preferredHelpWrapWidth},
		{width: 140, want: preferredHelpWrapWidth},
	}

	for _, tt := range tests {
		if got := helpWrapWidthForTerminalWidth(tt.width); got != tt.want {
			t.Fatalf("helpWrapWidthForTerminalWidth(%d) = %d, want %d", tt.width, got, tt.want)
		}
	}
}

func TestWriteWrappedTextWrapsWideOutput(t *testing.T) {
	var buf bytes.Buffer
	writeWrappedText(&buf, "This paragraph should wrap onto multiple lines when the configured help width is narrow enough.", 32, "")
	got := strings.TrimRight(buf.String(), "\n")
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output to span multiple lines, got:\n%s", got)
	}
	for _, line := range lines {
		if runeWidth(line) > 32 {
			t.Fatalf("expected line %q to fit within width 32", line)
		}
	}
}

func TestWriteWrappedTextLeavesNarrowOutputUnwrapped(t *testing.T) {
	var buf bytes.Buffer
	text := "This paragraph should remain on one long line when explicit wrapping is disabled."
	writeWrappedText(&buf, text, 0, "")
	if got := strings.TrimRight(buf.String(), "\n"); got != text {
		t.Fatalf("expected unwrapped output %q, got %q", text, got)
	}
}

func TestWriteCommandHelpWrapsCommandDescriptions(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "stack",
		Long:  "A command with a deliberately long description to exercise the help renderer.",
		Short: "root",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "submit",
		Short: "Push branches and create or update pull requests while keeping the output wrapped to a readable width.",
	})

	var buf bytes.Buffer
	writeCommandHelp(&buf, cmd, 48)
	got := buf.String()
	if !strings.Contains(got, "Available Commands:") {
		t.Fatalf("expected available commands section, got:\n%s", got)
	}
	if !strings.Contains(got, "submit") {
		t.Fatalf("expected command entry, got:\n%s", got)
	}
	if !strings.Contains(got, "Push branches and create or update") {
		t.Fatalf("expected first wrapped command description line, got:\n%s", got)
	}
	if !strings.Contains(got, "\n          pull requests while keeping the output") {
		t.Fatalf("expected wrapped continuation line for command description, got:\n%s", got)
	}
	if !strings.Contains(got, "\n          wrapped to a readable width.") {
		t.Fatalf("expected wrapped continuation line for command description, got:\n%s", got)
	}
}
