package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestSubmitQueueForTargetReturnsRootToTargetPath(t *testing.T) {
	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one":   &BranchRef{Parent: "main"},
			"feat-two":   &BranchRef{Parent: "feat-one"},
			"feat-three": &BranchRef{Parent: "feat-two"},
		},
	}

	got, err := submitQueue(state, false, []string{"feat-three"})
	if err != nil {
		t.Fatalf("submitQueue returned error: %v", err)
	}
	want := []string{"feat-one", "feat-two", "feat-three"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected queue: got=%v want=%v", got, want)
	}
}

func TestSubmitQueueAllReturnsTopologicalOrder(t *testing.T) {
	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one":   &BranchRef{Parent: "main"},
			"feat-two":   &BranchRef{Parent: "feat-one"},
			"feat-three": &BranchRef{Parent: "feat-two"},
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
	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": &BranchRef{Parent: "main"},
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
