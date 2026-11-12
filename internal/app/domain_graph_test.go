package app

import (
	"errors"
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

	got, err := topoOrderSelected(state, selected)
	if err != nil {
		t.Fatal(err)
	}
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

func TestSubmitQueueOrdersComponentsWithUntrackedParents(t *testing.T) {
	for iteration := 0; iteration < 50; iteration++ {
		state := &State{Trunk: "main", Branches: map[string]*BranchRef{
			"z-parent":     {Parent: "external"},
			"a-child":      {Parent: "z-parent"},
			"b-grandchild": {Parent: "a-child"},
			"other":        {Parent: "main"},
		}}
		got, err := submitQueue(state, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"other", "z-parent", "a-child", "b-grandchild"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("iteration %d: got %v, want %v", iteration, got, want)
		}
	}
}

func TestSubmitQueueRefusesInvalidGraphs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		branches map[string]*BranchRef
	}{
		{"self-parent", map[string]*BranchRef{"a": {Parent: "a"}}},
		{"cycle", map[string]*BranchRef{"a": {Parent: "b"}, "b": {Parent: "a"}}},
		{"missing-metadata", map[string]*BranchRef{"a": nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := submitQueue(&State{Trunk: "main", Branches: tc.branches}, true, nil)
			if err == nil {
				t.Fatal("expected graph validation error")
			}
			var graphErr *graphError
			if !errors.As(err, &graphErr) || graphErr.Branch == "" {
				t.Fatalf("expected structured graph error, got %v", err)
			}
		})
	}
}

func TestTopoOrderSelectedTreatsUnselectedParentsAsExternal(t *testing.T) {
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"parent": {Parent: "external"},
		"child":  {Parent: "parent"},
		"bad":    nil,
	}}
	got, err := topoOrderSelected(state, map[string]bool{"child": true})
	if err != nil || !reflect.DeepEqual(got, []string{"child"}) {
		t.Fatalf("selected queue = %v, %v", got, err)
	}
	if _, err := topoOrderSelected(state, map[string]bool{"absent": true}); err == nil {
		t.Fatal("expected missing selected metadata error")
	}
	got, err = topoOrder(&State{Trunk: "main"})
	if err != nil || len(got) != 0 {
		t.Fatalf("empty graph = %v, %v", got, err)
	}
}
