package app

import (
	"strings"
	"testing"
)

func TestValidateReparentParentRejectsSelfParent(t *testing.T) {
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{}}

	err := validateReparentParent(state, "feat-one", "feat-one")
	if err == nil {
		t.Fatalf("expected validation error for self parent")
	}
	if !strings.Contains(err.Error(), "branch cannot parent itself") {
		t.Fatalf("expected self-parent validation message, got: %v", err)
	}
}

func TestValidateReparentParentRejectsDescendantParent(t *testing.T) {
	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main", LineageParent: "main"},
			"feat-two": {Parent: "feat-one", LineageParent: "feat-one"},
		},
	}

	err := validateReparentParent(state, "feat-one", "feat-two")
	if err == nil {
		t.Fatalf("expected validation error when new parent is descendant")
	}
	if !strings.Contains(err.Error(), "parent cannot be a descendant") {
		t.Fatalf("expected descendant validation message, got: %v", err)
	}
}
