package app

import (
	"io"
	"reflect"
	"testing"
)

func TestOrderedSelectedLineageBranchesIncludesArchivedAncestors(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-child": {
				Parent:        "main",
				LineageParent: "feat-parent",
				PR:            &PRMeta{Number: 2},
			},
		},
		Archived: map[string]*ArchivedRef{
			"feat-parent": {
				Parent: "main",
				PR:     &PRMeta{Number: 1},
			},
		},
	}

	got := orderedSelectedLineageBranches(state, map[string]bool{"feat-child": true})
	want := []string{"feat-parent", "feat-child"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected lineage order: got=%v want=%v", got, want)
	}
}

func TestPruneArchivedLineageDropsUnreferencedNodes(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-live": {
				Parent:        "main",
				LineageParent: "feat-keep",
			},
		},
		Archived: map[string]*ArchivedRef{
			"feat-keep": {Parent: "main"},
			"feat-drop": {Parent: "main"},
		},
	}

	pruneArchivedLineage(state)
	if _, ok := state.Archived["feat-keep"]; !ok {
		t.Fatal("expected referenced archived node to be kept")
	}
	if _, ok := state.Archived["feat-drop"]; ok {
		t.Fatal("expected unreferenced archived node to be pruned")
	}
}

func TestReparentChildrenAfterMergedDeletionUpdatesParentAndPRBase(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {
				Parent: "main",
			},
			"feat-two": {
				Parent: "feat-one",
				PR:     &PRMeta{Number: 2, Base: "feat-one"},
			},
		},
	}

	reparentChildrenAfterMergedDeletion(state, "feat-one", "main", io.Discard)

	if got := state.Branches["feat-two"].Parent; got != "main" {
		t.Fatalf("expected feat-two parent main after clean, got %q", got)
	}
	if got := state.Branches["feat-two"].PR.Base; got != "main" {
		t.Fatalf("expected feat-two PR base main after clean, got %q", got)
	}
}
