package app

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var testCLIBinary string
var tempPathMu sync.Mutex
var tempPathRecords []string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "stack-git-config-")
	if err != nil {
		panic(err)
	}
	recordTempPath("testmain", tmp)
	repoRoot, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	repoRoot = filepath.Clean(filepath.Join(repoRoot, "..", ".."))
	global := filepath.Join(tmp, "global.gitconfig")
	if err := os.WriteFile(global, []byte{}, 0o600); err != nil {
		panic(err)
	}
	testCLIBinary = filepath.Join(tmp, "stack-test")
	recordTempPath("test-cli-binary", testCLIBinary)
	build := exec.Command("go", "build", "-o", testCLIBinary, "./cmd/stack")
	build.Dir = repoRoot
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}

	_ = os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	_ = os.Setenv("GIT_CONFIG_GLOBAL", global)
	_ = os.Setenv("GIT_TERMINAL_PROMPT", "0")

	timingLog := filepath.Join(tmp, "command-timings.log")
	if os.Getenv("STACK_TEST_COMMAND_TIMING") != "" {
		if err := enableCommandTimingWrappers(tmp, timingLog); err != nil {
			panic(err)
		}
	}

	code := m.Run()
	if os.Getenv("STACK_TEST_COMMAND_TIMING") != "" {
		summary := formatCommandTimingSummary(timingLog)
		fmt.Fprint(os.Stderr, summary)
		if summaryPath := os.Getenv("STACK_TEST_COMMAND_TIMING_SUMMARY"); summaryPath != "" {
			_ = os.WriteFile(summaryPath, []byte(summary), 0o600)
		}
	}
	if os.Getenv("STACK_TEST_TEMP_PATH_DIAGNOSTIC") != "" {
		summary := formatTempPathSummary()
		fmt.Fprint(os.Stderr, summary)
		if summaryPath := os.Getenv("STACK_TEST_TEMP_PATH_SUMMARY"); summaryPath != "" {
			_ = os.WriteFile(summaryPath, []byte(summary), 0o600)
		}
	}
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

func recordTempPath(label, path string) {
	if os.Getenv("STACK_TEST_TEMP_PATH_DIAGNOSTIC") == "" {
		return
	}
	tempPathMu.Lock()
	defer tempPathMu.Unlock()
	tempPathRecords = append(tempPathRecords, label+"\t"+path)
}

func formatTempPathSummary() string {
	tempPathMu.Lock()
	records := append([]string(nil), tempPathRecords...)
	tempPathMu.Unlock()
	sort.Strings(records)
	var out strings.Builder
	tmpRoot := os.Getenv("TMPDIR")
	if tmpRoot == "" {
		tmpRoot = os.TempDir()
	}
	fmt.Fprintf(&out, "temp path summary:\n")
	fmt.Fprintf(&out, "  TMPDIR=%s\n", tmpRoot)
	for _, record := range records {
		parts := strings.SplitN(record, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		location := "outside-tmpdir"
		if strings.HasPrefix(parts[1], tmpRoot) {
			location = "under-tmpdir"
		}
		fmt.Fprintf(&out, "  %s [%s] %s\n", parts[0], location, parts[1])
	}
	return out.String()
}

func enableCommandTimingWrappers(tmpDir, logPath string) error {
	wrapperDir := filepath.Join(tmpDir, "command-wrappers")
	if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"git", "gh"} {
		realPath, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if err := writeCommandTimingWrapper(filepath.Join(wrapperDir, name), name, realPath, logPath); err != nil {
			return err
		}
	}
	pathValue := wrapperDir
	if current := os.Getenv("PATH"); current != "" {
		pathValue += string(os.PathListSeparator) + current
	}
	return os.Setenv("PATH", pathValue)
}

func writeCommandTimingWrapper(path, name, realPath, logPath string) error {
	script := fmt.Sprintf("#!/bin/sh\nREAL_CMD=%s\nLOG_PATH=%s\nstart=$(perl -MTime::HiRes=time -e 'printf \"%%.6f\", time')\n\"$REAL_CMD\" \"$@\"\nstatus=$?\nend=$(perl -MTime::HiRes=time -e 'printf \"%%.6f\", time')\nperl -e 'my ($start, $end, $name, @args) = @ARGV; my $cmd = join(\" \", @args); $cmd =~ s/\\t/ /g; printf \"%%s\\t%%.6f\\t%%s\\n\", $name, $end - $start, $cmd;' \"$start\" \"$end\" %s \"$@\" >> \"$LOG_PATH\"\nexit $status\n", strconv.Quote(realPath), strconv.Quote(logPath), strconv.Quote(name))
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return err
	}
	return nil
}

type commandTimingSummary struct {
	count int
	total float64
}

type commandSignatureSummary struct {
	name  string
	count int
	total float64
}

func formatCommandTimingSummary(logPath string) string {
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "command timing summary: no subprocess timings recorded\n"
		}
		return fmt.Sprintf("command timing summary: %v\n", err)
	}
	defer file.Close()
	var out strings.Builder

	byCommand := map[string]*commandTimingSummary{}
	bySignature := map[string]*commandSignatureSummary{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 3)
		if len(parts) != 3 {
			continue
		}
		duration, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}
		commandName := parts[0]
		signature := strings.TrimSpace(parts[2])
		if byCommand[commandName] == nil {
			byCommand[commandName] = &commandTimingSummary{}
		}
		byCommand[commandName].count++
		byCommand[commandName].total += duration
		key := commandName + "\t" + signature
		if bySignature[key] == nil {
			bySignature[key] = &commandSignatureSummary{name: key}
		}
		bySignature[key].count++
		bySignature[key].total += duration
	}
	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("command timing summary: %v\n", err)
	}

	commandNames := make([]string, 0, len(byCommand))
	for name := range byCommand {
		commandNames = append(commandNames, name)
	}
	sort.Strings(commandNames)

	out.WriteString("command timing summary:\n")
	for _, name := range commandNames {
		summary := byCommand[name]
		fmt.Fprintf(&out, "  %s calls=%d total=%.3fs\n", name, summary.count, summary.total)
	}

	signatures := make([]commandSignatureSummary, 0, len(bySignature))
	for _, summary := range bySignature {
		signatures = append(signatures, *summary)
	}
	sort.Slice(signatures, func(i, j int) bool {
		if signatures[i].total == signatures[j].total {
			return signatures[i].name < signatures[j].name
		}
		return signatures[i].total > signatures[j].total
	})

	limit := 10
	if len(signatures) < limit {
		limit = len(signatures)
	}
	for i := 0; i < limit; i++ {
		parts := strings.SplitN(signatures[i].name, "\t", 2)
		fmt.Fprintf(&out, "  top[%d] %s total=%.3fs count=%d cmd=%s\n", i+1, parts[0], signatures[i].total, signatures[i].count, parts[1])
	}
	return out.String()
}
