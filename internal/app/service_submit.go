package app

import (
	"sort"
	"strings"
	"sync"
)

type submitDeps struct {
	git                  submitGitClient
	gh                   submitGHClient
	ensureCleanWorktree  func() error
	loadState            func() (string, *State, bool, error)
	submitQueue          func(*State, bool, []string) ([]string, error)
	ensurePR             func(string, string, *PRMeta, *GhPR) (*PRMeta, error)
	syncCurrentStackBody func(*State, bool, string) error
	saveState            func(string, *State) error
	cleanupMergedBranch  func(*State, string, string) (bool, error)
}

func ensurePR(branch, parent string, existing *PRMeta, existingPR *GhPR) (*PRMeta, error) {
	latestTitle, summary, err := branchSummary(parent, branch)
	if err != nil {
		return nil, err
	}
	defaultBody := composeBody(summary, "")

	if existing != nil && existing.Number > 0 {
		if existingPR == nil {
			pr, err := ghView(existing.Number)
			if err == nil {
				existingPR = pr
			}
		}
		if existingPR != nil && strings.EqualFold(existingPR.State, "OPEN") {
			if err := ghEdit(existing.Number, parent, existingPR.Body); err != nil {
				return nil, err
			}
			return &PRMeta{Number: existing.Number, URL: existingPR.URL, Base: parent, Updated: true}, nil
		}
	}

	if open, err := ghFindByHead(branch); err == nil && open != nil {
		if err := ghEdit(open.Number, parent, open.Body); err != nil {
			return nil, err
		}
		return &PRMeta{Number: open.Number, URL: open.URL, Base: parent, Updated: true}, nil
	}

	number, url, err := ghCreate(branch, parent, latestTitle, defaultBody)
	if err != nil {
		return nil, err
	}
	return &PRMeta{Number: number, URL: url, Base: parent, Updated: false}, nil
}

func syncCurrentStackBodies(state *State, all bool, target string) error {
	selected, err := stackSelection(state, all, target)
	if err != nil {
		return err
	}
	ordered := orderedSelectedLineageBranches(state, selected)
	snapshots := fetchStackBodySyncSnapshots(state, ordered)

	lines := []StackPRLine{}
	updates := []stackBodyUpdate{}
	for _, snapshot := range snapshots {
		if snapshot.hasLine {
			lines = append(lines, snapshot.line)
		}
		if snapshot.hasUpdate {
			updates = append(updates, snapshot.update)
		}
	}

	if len(lines) == 0 || len(updates) == 0 {
		return nil
	}

	return applyStackBodyUpdates(lines, updates)
}

type stackBodyUpdate struct {
	branch string
	number int
	base   string
	body   string
}

type stackBodySyncSnapshot struct {
	line      StackPRLine
	hasLine   bool
	update    stackBodyUpdate
	hasUpdate bool
}

func fetchStackBodySyncSnapshots(state *State, ordered []string) []stackBodySyncSnapshot {
	results := make([]stackBodySyncSnapshot, len(ordered))

	var fetchWG sync.WaitGroup
	fetchWG.Add(len(ordered))
	for i, branch := range ordered {
		go func(idx int, branch string) {
			defer fetchWG.Done()
			prMeta := lineagePRMeta(state, branch)
			if prMeta == nil || prMeta.Number <= 0 {
				return
			}
			pr, err := ghView(prMeta.Number)
			if err != nil {
				return
			}

			snapshot := stackBodySyncSnapshot{
				line: StackPRLine{
					Branch: branch,
					Number: pr.Number,
					Title:  pr.Title,
					URL:    pr.URL,
					State:  pr.State,
				},
				hasLine: true,
			}

			if strings.EqualFold(pr.State, "OPEN") {
				meta := state.Branches[branch]
				if meta != nil {
					base := meta.Parent
					if base == "" {
						base = state.Trunk
					}
					snapshot.update = stackBodyUpdate{branch: branch, number: pr.Number, base: base, body: pr.Body}
					snapshot.hasUpdate = true
				}
			}

			results[idx] = snapshot
		}(i, branch)
	}
	fetchWG.Wait()
	return results
}

func applyStackBodyUpdates(lines []StackPRLine, updates []stackBodyUpdate) error {
	var editWG sync.WaitGroup
	errCh := make(chan error, len(updates))

	editWG.Add(len(updates))
	for _, update := range updates {
		go func(update stackBodyUpdate) {
			defer editWG.Done()
			managed := managedStackBlock(update.branch, lines)
			body := upsertManagedBlock(update.body, managed)
			if err := ghEdit(update.number, update.base, body); err != nil {
				errCh <- err
			}
		}(update)
	}
	editWG.Wait()
	close(errCh)

	for err := range errCh {
		return err
	}
	return nil
}

func stackSelection(state *State, all bool, target string) (map[string]bool, error) {
	selected := map[string]bool{}
	if all {
		for branch := range state.Branches {
			selected[branch] = true
		}
		return selected, nil
	}
	if target == "" {
		current, err := currentBranch()
		if err != nil {
			return nil, err
		}
		target = current
	}
	return branchesInCurrentStack(state, target), nil
}

func orderedSelectedBranches(state *State, selected map[string]bool) []string {
	return orderedSelectedLineageBranches(state, selected)
}

func orderedSelectedLineageBranches(state *State, selected map[string]bool) []string {
	if len(selected) == 0 {
		return nil
	}

	included := map[string]bool{}
	for branch := range selected {
		included[branch] = true
		collectLineageAncestors(state, branch, included)
	}

	children := map[string][]string{}
	for branch := range state.Branches {
		if !included[branch] {
			continue
		}
		parent := lineageParent(state, branch)
		if parent == "" {
			parent = state.Trunk
		}
		children[parent] = append(children[parent], branch)
	}
	for branch, meta := range state.Archived {
		if !included[branch] || meta == nil {
			continue
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}
		children[parent] = append(children[parent], branch)
	}
	for parent := range children {
		sort.Strings(children[parent])
	}

	ordered := []string{}
	seen := map[string]bool{}
	var visit func(parent string)
	visit = func(parent string) {
		for _, child := range children[parent] {
			if seen[child] {
				continue
			}
			seen[child] = true
			ordered = append(ordered, child)
			visit(child)
		}
	}

	visit(state.Trunk)
	for branch := range included {
		if seen[branch] {
			continue
		}
		ordered = append(ordered, branch)
	}
	return ordered
}

func collectLineageAncestors(state *State, branch string, included map[string]bool) {
	cur := lineageParent(state, branch)
	seen := map[string]bool{}
	for cur != "" && cur != state.Trunk {
		if seen[cur] {
			break
		}
		seen[cur] = true
		included[cur] = true
		cur = lineageParent(state, cur)
	}
}

func lineageParent(state *State, branch string) string {
	if meta := state.Branches[branch]; meta != nil {
		if meta.LineageParent != "" {
			return meta.LineageParent
		}
		return meta.Parent
	}
	if meta := state.Archived[branch]; meta != nil {
		return meta.Parent
	}
	return ""
}

func lineagePRMeta(state *State, branch string) *PRMeta {
	if meta := state.Branches[branch]; meta != nil {
		return meta.PR
	}
	if meta := state.Archived[branch]; meta != nil {
		return meta.PR
	}
	return nil
}
