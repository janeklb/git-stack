package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type doctorReport struct {
	out      io.Writer
	errors   int
	warnings int
	infos    int
}

type doctorSeverity int

const (
	doctorSeverityError doctorSeverity = iota
	doctorSeverityWarn
	doctorSeverityInfo
)

func (s doctorSeverity) String() string {
	switch s {
	case doctorSeverityError:
		return "ERROR"
	case doctorSeverityWarn:
		return "WARN"
	case doctorSeverityInfo:
		return "INFO"
	default:
		return "UNKNOWN"
	}
}

func (a *App) cmdDoctor() error {
	report := doctorReport{out: a.stdout}

	repoRoot, state, err := loadStateFromRepo()
	if err != nil {
		if errors.Is(err, errStateNotInitialized) {
			report.add(doctorSeverityError, "state-not-initialized", "hint=run-stack-init")
		} else {
			report.add(doctorSeverityError, "state-unreadable", fmt.Sprintf("detail=%q", err.Error()))
		}
		report.printSummary()
		return report.exitError()
	}

	if strings.TrimSpace(state.Trunk) == "" || !branchExists(state.Trunk) {
		report.add(doctorSeverityError, "trunk-missing", fmt.Sprintf("trunk=%s", strings.TrimSpace(state.Trunk)))
	}

	for _, branch := range sortedBranchNames(state.Branches) {
		meta := state.Branches[branch]
		if meta == nil {
			report.add(doctorSeverityError, "branch-metadata-missing", fmt.Sprintf("branch=%s", branch))
			continue
		}
		parent := strings.TrimSpace(meta.Parent)
		if parent == "" {
			report.add(doctorSeverityError, "missing-parent", fmt.Sprintf("branch=%s", branch))
			continue
		}
		if !branchExists(parent) {
			report.add(doctorSeverityError, "parent-missing", fmt.Sprintf("branch=%s", branch), fmt.Sprintf("parent=%s", parent))
			continue
		}
		if err := validateReparentParent(state, branch, parent); err != nil {
			report.add(doctorSeverityError, "cycle-detected", fmt.Sprintf("branch=%s", branch), fmt.Sprintf("parent=%s", parent))
			continue
		}
		if err := gitRun("merge-base", "--is-ancestor", parent, branch); err != nil {
			report.add(doctorSeverityError, "parent-not-ancestor", fmt.Sprintf("branch=%s", branch), fmt.Sprintf("parent=%s", parent))
		}
	}

	rooted := rootedBranches(state)
	for _, branch := range sortedBranchNames(state.Branches) {
		if rooted[branch] {
			continue
		}
		parent := ""
		if meta := state.Branches[branch]; meta != nil {
			parent = strings.TrimSpace(meta.Parent)
		}
		report.add(doctorSeverityWarn, "unrooted-branch", fmt.Sprintf("branch=%s", branch), fmt.Sprintf("parent=%s", parent))
	}

	op, opErr := loadOperation(repoRoot)
	if opErr != nil {
		if !errors.Is(opErr, os.ErrNotExist) {
			report.add(doctorSeverityWarn, "restack-operation-unreadable", fmt.Sprintf("detail=%q", opErr.Error()))
		}
	} else if op != nil {
		if op.Index < 0 || op.Index > len(op.Queue) || strings.TrimSpace(op.Mode) == "" || strings.TrimSpace(op.OriginalBranch) == "" {
			report.add(doctorSeverityWarn, "restack-operation-stale", fmt.Sprintf("index=%d", op.Index), fmt.Sprintf("queue=%d", len(op.Queue)))
		} else {
			report.add(doctorSeverityWarn, "restack-operation-present", fmt.Sprintf("mode=%s", op.Mode), fmt.Sprintf("index=%d", op.Index), fmt.Sprintf("queue=%d", len(op.Queue)))
		}
	}

	branches, branchErr := listLocalBranches()
	if branchErr != nil {
		report.add(doctorSeverityError, "local-branches-unreadable", fmt.Sprintf("detail=%q", branchErr.Error()))
	} else {
		sort.Strings(branches)
		for _, branch := range branches {
			if branch == state.Trunk {
				continue
			}
			if _, ok := state.Branches[branch]; ok {
				continue
			}
			report.add(doctorSeverityInfo, "missing-state-entry", fmt.Sprintf("branch=%s", branch))
		}
	}

	report.printSummary()
	return report.exitError()
}

func (r *doctorReport) add(severity doctorSeverity, code string, fields ...string) {
	line := severity.String() + " " + code
	if len(fields) > 0 {
		line += " " + strings.Join(fields, " ")
	}
	fmt.Fprintln(r.out, line)
	switch severity {
	case doctorSeverityError:
		r.errors++
	case doctorSeverityWarn:
		r.warnings++
	case doctorSeverityInfo:
		r.infos++
	}
}

func (r *doctorReport) printSummary() {
	fmt.Fprintf(r.out, "%d errors, %d warnings, %d infos\n", r.errors, r.warnings, r.infos)
}

func (r *doctorReport) exitError() error {
	if r.errors == 0 {
		return nil
	}
	return fmt.Errorf("doctor found %d error(s)", r.errors)
}

func sortedBranchNames(branches map[string]*BranchRef) []string {
	names := make([]string, 0, len(branches))
	for name := range branches {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func rootedBranches(state *State) map[string]bool {
	rooted := map[string]bool{}
	children := map[string][]string{}
	for branch, meta := range state.Branches {
		if meta == nil {
			continue
		}
		children[meta.Parent] = append(children[meta.Parent], branch)
	}

	stack := []string{state.Trunk}
	for len(stack) > 0 {
		parent := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, child := range children[parent] {
			if rooted[child] {
				continue
			}
			rooted[child] = true
			stack = append(stack, child)
		}
	}
	return rooted
}
