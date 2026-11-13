package app

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type commandRunResult struct {
	stdout   string
	stderr   string
	exitCode int
}

type commandBoxMode int

const (
	commandBoxAlways commandBoxMode = iota
	commandBoxOnFailure
)

type commandRunOptions struct {
	streamOutput bool
	boxMode      commandBoxMode
	env          []string // Overrides applied to the inherited environment.
	dir          string
	stdin        io.Reader
	stdout       io.Writer
	stderr       io.Writer
	baseEnv      []string // nil inherits the process environment.
}

type commandRunner func(string, []string, commandRunOptions) (commandRunResult, error)

func mergeEnvironment(base, overrides []string) []string {
	merged := make([]string, 0, len(base)+len(overrides))
	positions := map[string]int{}
	for _, entries := range [][]string{base, overrides} {
		for _, entry := range entries {
			key, _, _ := strings.Cut(entry, "=")
			if runtime.GOOS == "windows" {
				key = strings.ToUpper(key)
			}
			if index, ok := positions[key]; ok {
				merged[index] = entry
			} else {
				positions[key] = len(merged)
				merged = append(merged, entry)
			}
		}
	}
	return merged
}

func runCommand(name string, args []string, opts commandRunOptions) (commandRunResult, error) {
	if opts.stdout == nil {
		opts.stdout = os.Stdout
	}
	if opts.stderr == nil {
		opts.stderr = os.Stderr
	}
	decorate := stdoutIsTTY(opts.stdout) || stdoutIsTTY(opts.stderr)
	cmd := exec.Command(name, args...)
	cmd.Dir = opts.dir
	cmd.Stdin = opts.stdin
	base := cmd.Environ()
	if opts.baseEnv != nil {
		base = opts.baseEnv
	}
	if opts.baseEnv != nil || opts.env != nil {
		cmd.Env = mergeEnvironment(base, opts.env)
		// exec.Command searches the process PATH before cmd.Env is assigned.
		// Explicit environments must also control which executable is run.
		cmd.Path, cmd.Err = executableInEnvironment(name, opts.dir, cmd.Env)
	}
	if !decorate && opts.streamOutput {
		return runCommandPassthrough(cmd, opts.stdout, opts.stderr)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return commandRunResult{exitCode: -1}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return commandRunResult{exitCode: -1}, err
	}

	theme := subprocessTheme{useColor: decorate}
	showLiveBox := decorate && opts.boxMode == commandBoxAlways
	if showLiveBox {
		fmt.Fprintln(opts.stdout, theme.header("┌─ "+formatCommand(name, args)))
	}

	if err := cmd.Start(); err != nil {
		if showLiveBox {
			fmt.Fprintln(opts.stdout, theme.footer("└─ -1"))
		}
		return commandRunResult{exitCode: -1}, err
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	writeMu := &sync.Mutex{}

	wg.Add(2)
	go func() {
		defer wg.Done()
		if opts.streamOutput && showLiveBox {
			copyDecoratedOutput(stdoutPipe, &stdoutBuf, opts.stdout, theme, theme.stdoutLine, writeMu)
			return
		}
		_, _ = io.Copy(&stdoutBuf, stdoutPipe)
	}()

	go func() {
		defer wg.Done()
		// Keep stderr buffered so we can choose its tone based on exit code.
		_, _ = io.Copy(&stderrBuf, stderrPipe)
	}()

	wg.Wait()
	waitErr := cmd.Wait()
	exitCode := exitCodeForError(waitErr)

	if showLiveBox {
		if opts.streamOutput {
			stderrStyle := theme.stdoutLine
			if waitErr != nil {
				stderrStyle = theme.stderrLine
			}
			printCapturedOutput(stderrBuf.String(), opts.stderr, theme, stderrStyle)
		}
		fmt.Fprintln(opts.stdout, theme.footer(fmt.Sprintf("└─ %d", exitCode)))
	} else if decorate && opts.boxMode == commandBoxOnFailure && waitErr != nil {
		hasCapturedOutput := strings.TrimSpace(stdoutBuf.String()) != "" || strings.TrimSpace(stderrBuf.String()) != ""
		if hasCapturedOutput {
			fmt.Fprintln(opts.stdout, theme.header("┌─ "+formatCommand(name, args)))
			if opts.streamOutput {
				printCapturedOutput(stdoutBuf.String(), opts.stdout, theme, theme.stdoutLine)
				printCapturedOutput(stderrBuf.String(), opts.stderr, theme, theme.stderrLine)
			}
			fmt.Fprintln(opts.stdout, theme.footer(fmt.Sprintf("└─ %d", exitCode)))
		}
	}

	result := commandRunResult{stdout: stdoutBuf.String(), stderr: stderrBuf.String(), exitCode: exitCode}
	if waitErr != nil {
		return result, waitErr
	}
	return result, nil
}

func executableInEnvironment(name, dir string, env []string) (string, error) {
	if strings.ContainsAny(name, `/\`) {
		return name, nil
	}
	path := ""
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if key == "PATH" || (runtime.GOOS == "windows" && strings.EqualFold(key, "PATH")) {
			path = value
		}
	}
	for _, entry := range filepath.SplitList(path) {
		candidate := entry
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(dir, candidate)
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			return "", err
		}
		if found, err := exec.LookPath(filepath.Join(absolute, name)); err == nil {
			// Preserve Go's refusal to resolve executables via relative PATHs.
			if !filepath.IsAbs(entry) {
				return "", &exec.Error{Name: name, Err: exec.ErrDot}
			}
			return found, nil
		}
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

func printCapturedOutput(captured string, dest io.Writer, theme subprocessTheme, styleFn func(string) string) {
	if captured == "" {
		return
	}
	writeDecoratedSegments(strings.NewReader(captured), nil, dest, theme, styleFn, nil)
}

func runCommandPassthrough(cmd *exec.Cmd, stdout, stderr io.Writer) (commandRunResult, error) {
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(stderr, &stderrBuf)
	err := cmd.Run()
	result := commandRunResult{stdout: stdoutBuf.String(), stderr: stderrBuf.String(), exitCode: exitCodeForError(err)}
	if err != nil {
		return result, err
	}
	return result, nil
}

func copyDecoratedOutput(src io.Reader, capture io.Writer, dest io.Writer, theme subprocessTheme, styleFn func(string) string, mu *sync.Mutex) {
	writeDecoratedSegments(src, capture, dest, theme, styleFn, mu)
}

func writeDecoratedSegments(src io.Reader, capture io.Writer, dest io.Writer, theme subprocessTheme, styleFn func(string) string, mu *sync.Mutex) {
	reader := bufio.NewReader(src)
	var segment bytes.Buffer
	for {
		b, err := reader.ReadByte()
		if err == nil {
			if capture != nil {
				_, _ = capture.Write([]byte{b})
			}
			switch b {
			case '\r':
				flushDecoratedSegment(segment.String(), dest, theme, styleFn, mu)
				segment.Reset()
			case '\n':
				flushDecoratedSegment(segment.String(), dest, theme, styleFn, mu)
				segment.Reset()
			default:
				segment.WriteByte(b)
			}
			continue
		}
		if err == io.EOF {
			flushDecoratedSegment(segment.String(), dest, theme, styleFn, mu)
			return
		}
		return
	}
}

func flushDecoratedSegment(segment string, dest io.Writer, theme subprocessTheme, styleFn func(string) string, mu *sync.Mutex) {
	if segment == "" {
		return
	}
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	_, _ = io.WriteString(dest, theme.formatLine(segment, styleFn)+"\n")
}

func exitCodeForError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func formatCommand(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, formatCommandPart(name))
	for _, arg := range args {
		parts = append(parts, formatCommandPart(arg))
	}
	return strings.Join(parts, " ")
}

func formatCommandPart(part string) string {
	if part == "" {
		return "\"\""
	}
	if strings.ContainsAny(part, " \t\n\"'") {
		return strconv.Quote(part)
	}
	return part
}

type subprocessTheme struct {
	useColor bool
}

func (t subprocessTheme) header(text string) string {
	return t.wrap(text, "36")
}

func (t subprocessTheme) footer(text string) string {
	return t.wrap(text, "36")
}

func (t subprocessTheme) stdoutLine(text string) string {
	return t.wrap(text, "90")
}

func (t subprocessTheme) stderrLine(text string) string {
	return t.wrap(text, "31")
}

func (t subprocessTheme) wrap(text, code string) string {
	if !t.useColor {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (t subprocessTheme) formatLine(text string, styleFn func(string) string) string {
	if !t.useColor {
		return "│ " + text
	}
	return t.header("│") + " " + styleFn(text)
}
