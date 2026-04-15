package app

import (
	"bytes"
	"testing"
)

func TestSubprocessThemeFormatLineKeepsBorderColor(t *testing.T) {
	t.Parallel()

	theme := subprocessTheme{useColor: true}
	got := theme.formatLine("hello", theme.stderrLine)
	want := "\x1b[36m│\x1b[0m \x1b[31mhello\x1b[0m"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSubprocessThemeFormatLineWithoutColor(t *testing.T) {
	t.Parallel()

	theme := subprocessTheme{useColor: false}
	got := theme.formatLine("hello", theme.stdoutLine)
	want := "│ hello"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPrintCapturedOutputTreatsCarriageReturnAsBoundary(t *testing.T) {
	t.Parallel()

	theme := subprocessTheme{useColor: false}
	var out bytes.Buffer
	printCapturedOutput("Rebasing (1/1)\rSuccessfully rebased\n", &out, theme, theme.stdoutLine)

	want := "│ Rebasing (1/1)\n│ Successfully rebased\n"
	if out.String() != want {
		t.Fatalf("expected %q, got %q", want, out.String())
	}
}
