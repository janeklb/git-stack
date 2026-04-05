package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestSubmitQueueForTargetReturnsCurrentStackTreeOrder(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one":   {Parent: "main"},
			"feat-two":   {Parent: "feat-one"},
			"feat-three": {Parent: "feat-two"},
			"feat-side":  {Parent: "feat-one"},
			"other-root": {Parent: "main"},
		},
	}

	got, err := submitQueue(state, false, []string{"feat-three"})
	if err != nil {
		t.Fatalf("submitQueue returned error: %v", err)
	}
	want := []string{"feat-one", "feat-side", "feat-two", "feat-three"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected queue: got=%v want=%v", got, want)
	}
}

func TestSubmitQueueAllReturnsTopologicalOrder(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one":   {Parent: "main"},
			"feat-two":   {Parent: "feat-one"},
			"feat-three": {Parent: "feat-two"},
		},
	}

	got, err := submitQueue(state, true, nil)
	if err != nil {
		t.Fatalf("submitQueue returned error: %v", err)
	}
	want := []string{"feat-one", "feat-two", "feat-three"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected queue: got=%v want=%v", got, want)
	}
}

func TestSubmitQueueErrorsForUnknownTarget(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main"},
		},
	}

	_, err := submitQueue(state, false, []string{"missing"})
	if err == nil {
		t.Fatal("expected submitQueue to fail for unknown branch")
	}
	if !strings.Contains(err.Error(), "branch not tracked in stack") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTopoOrderSelectedRestrictsToChosenBranches(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one":   {Parent: "main"},
			"feat-two":   {Parent: "feat-one"},
			"feat-three": {Parent: "feat-two"},
			"feat-side":  {Parent: "feat-one"},
			"other-root": {Parent: "main"},
		},
	}

	selected := map[string]bool{
		"feat-one":   true,
		"feat-two":   true,
		"feat-three": true,
		"feat-side":  true,
	}

	got := topoOrderSelected(state, selected)
	want := []string{"feat-one", "feat-side", "feat-two", "feat-three"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected selected topo order: got=%v want=%v", got, want)
	}
}

func TestBranchesInCurrentStackReturnsOnlyConnectedStack(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"stack-a-1": {Parent: "main"},
			"stack-a-2": {Parent: "stack-a-1"},
			"stack-b-1": {Parent: "main"},
		},
	}

	selected := branchesInCurrentStack(state, "stack-a-2")
	if !selected["stack-a-1"] || !selected["stack-a-2"] {
		t.Fatalf("expected connected stack branches selected, got: %#v", selected)
	}
	if selected["stack-b-1"] {
		t.Fatalf("did not expect unrelated stack branch selected, got: %#v", selected)
	}
}

func TestBranchesInCurrentStackTrunkSelectsAllBranches(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main"},
			"feat-two": {Parent: "feat-one"},
		},
	}

	selected := branchesInCurrentStack(state, "main")
	if !selected["feat-one"] || !selected["feat-two"] {
		t.Fatalf("expected all branches selected from trunk, got: %#v", selected)
	}
}
