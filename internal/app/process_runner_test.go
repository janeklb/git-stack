package app

import (
	"bytes"
	"os"
	"reflect"
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

func TestMergeEnvironmentOverridesWithoutDuplicateKeys(t *testing.T) {
	got := mergeEnvironment([]string{"PATH=/bin", "HOME=/home/test", "EDITOR=old"}, []string{"EDITOR=new", "EMPTY=", "EDITOR=last"})
	want := []string{"PATH=/bin", "HOME=/home/test", "EDITOR=last", "EMPTY="}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
}

func TestCommandEditorOverridesPreserveInheritedEnvironment(t *testing.T) {
	t.Setenv("STACK_TEST_ENV", "inherited")
	t.Setenv("GIT_EDITOR", "must-not-run")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("STACK_TEST_EMPTY", "not empty")
	args := []string{"-c", `printf '%s\n' "$STACK_TEST_ENV" "$PATH" "$HOME" "$GIT_CONFIG_COUNT" "$GIT_EDITOR" "$GIT_MERGE_AUTOEDIT" "$STACK_TEST_EMPTY"`}
	overrides := []string{"GIT_EDITOR=true", "GIT_MERGE_AUTOEDIT=no", "STACK_TEST_EMPTY="}
	want := "inherited\n" + os.Getenv("PATH") + "\n" + os.Getenv("HOME") + "\n0\ntrue\nno\n\n"
	for _, passthrough := range []bool{false, true} {
		var result commandRunResult
		var err error
		if passthrough {
			result, err = runCommandPassthroughWithEnv("sh", args, overrides)
		} else {
			result, err = runCommand("sh", args, commandRunOptions{env: overrides})
		}
		if err != nil {
			t.Fatal(err)
		}
		if result.stdout != want {
			t.Fatalf("passthrough=%v: environment output = %q, want %q", passthrough, result.stdout, want)
		}
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
