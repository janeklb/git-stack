package app

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
}

func runCommand(name string, args []string, opts commandRunOptions) (commandRunResult, error) {
	if !shouldDecorateSubprocessOutput() && opts.streamOutput {
		return runCommandPassthrough(name, args)
	}

	cmd := exec.Command(name, args...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return commandRunResult{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return commandRunResult{}, err
	}

	decorate := shouldDecorateSubprocessOutput()
	theme := subprocessTheme{useColor: decorate}
	showLiveBox := decorate && opts.boxMode == commandBoxAlways
	if showLiveBox {
		fmt.Fprintln(os.Stdout, theme.header("┌─ "+formatCommand(name, args)))
	}

	if err := cmd.Start(); err != nil {
		if showLiveBox {
			fmt.Fprintln(os.Stdout, theme.footer("└─ -1"))
		}
		return commandRunResult{}, err
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	writeMu := &sync.Mutex{}

	wg.Add(2)
	go func() {
		defer wg.Done()
		if opts.streamOutput && showLiveBox {
			copyDecoratedOutput(stdoutPipe, &stdoutBuf, os.Stdout, theme, theme.stdoutLine, writeMu)
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
			printCapturedOutput(stderrBuf.String(), os.Stderr, theme, stderrStyle)
		}
		fmt.Fprintf(os.Stdout, "%s\n", theme.footer(fmt.Sprintf("└─ %d", exitCode)))
	} else if decorate && opts.boxMode == commandBoxOnFailure && waitErr != nil {
		hasCapturedOutput := strings.TrimSpace(stdoutBuf.String()) != "" || strings.TrimSpace(stderrBuf.String()) != ""
		if hasCapturedOutput {
			fmt.Fprintln(os.Stdout, theme.header("┌─ "+formatCommand(name, args)))
			if opts.streamOutput {
				printCapturedOutput(stdoutBuf.String(), os.Stdout, theme, theme.stdoutLine)
				printCapturedOutput(stderrBuf.String(), os.Stderr, theme, theme.stderrLine)
			}
			fmt.Fprintf(os.Stdout, "%s\n", theme.footer(fmt.Sprintf("└─ %d", exitCode)))
		}
	}

	result := commandRunResult{stdout: stdoutBuf.String(), stderr: stderrBuf.String(), exitCode: exitCode}
	if waitErr != nil {
		return result, waitErr
	}
	return result, nil
}

func printCapturedOutput(captured string, dest io.Writer, theme subprocessTheme, styleFn func(string) string) {
	if captured == "" {
		return
	}
	reader := bufio.NewReader(strings.NewReader(captured))
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSuffix(line, "\n")
			_, _ = io.WriteString(dest, theme.formatLine(trimmed, styleFn)+"\n")
		}
		if err == nil {
			continue
		}
		if err == io.EOF {
			return
		}
		return
	}
}

func runCommandPassthrough(name string, args []string) (commandRunResult, error) {
	cmd := exec.Command(name, args...)
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	err := cmd.Run()
	result := commandRunResult{stdout: stdoutBuf.String(), stderr: stderrBuf.String(), exitCode: exitCodeForError(err)}
	if err != nil {
		return result, err
	}
	return result, nil
}

func copyDecoratedOutput(src io.Reader, capture io.Writer, dest io.Writer, theme subprocessTheme, styleFn func(string) string, mu *sync.Mutex) {
	reader := bufio.NewReader(src)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			_, _ = io.WriteString(capture, line)
			trimmed := strings.TrimSuffix(line, "\n")
			mu.Lock()
			_, _ = io.WriteString(dest, theme.formatLine(trimmed, styleFn)+"\n")
			mu.Unlock()
		}
		if err == nil {
			continue
		}
		if err == io.EOF {
			return
		}
		return
	}
}

func shouldDecorateSubprocessOutput() bool {
	return stdoutIsTTY(os.Stdout) || stdoutIsTTY(os.Stderr)
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
