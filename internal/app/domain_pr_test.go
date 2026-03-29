package app

import (
	"strings"
	"testing"
)

func TestUpsertManagedBlockAppendsAndReplaces(t *testing.T) {
	managedOne := managedStackBlock("feat-one", []StackPRLine{{
		Branch: "feat-one",
		Number: 10,
		Title:  "Feature one",
		URL:    "https://example.com/pr/10",
		State:  "OPEN",
	}})
	body := upsertManagedBlock("User body", managedOne)
	if !strings.Contains(body, "User body") {
		t.Fatalf("expected original body to be preserved, got:\n%s", body)
	}
	if !strings.Contains(body, "## Current Stack") {
		t.Fatalf("expected managed block to be appended, got:\n%s", body)
	}

	managedTwo := managedStackBlock("feat-one", []StackPRLine{{
		Branch: "feat-zero",
		Number: 9,
		Title:  "Feature zero",
		URL:    "https://example.com/pr/9",
		State:  "MERGED",
	}})
	replaced := upsertManagedBlock(body, managedTwo)
	if strings.Contains(replaced, "#10 Feature one") {
		t.Fatalf("expected old managed block to be replaced, got:\n%s", replaced)
	}
	if !strings.Contains(replaced, "#9 Feature zero") {
		t.Fatalf("expected new managed block, got:\n%s", replaced)
	}
}

func TestUpsertManagedBlockPreservesTextOutsideManagedSection(t *testing.T) {
	body := strings.Join([]string{
		"Intro",
		managedBlockStart,
		"old",
		managedBlockEnd,
		"Outro",
	}, "\n")
	updated := upsertManagedBlock(body, managedStackBlock("feat-a", []StackPRLine{{
		Branch: "feat-a",
		Number: 11,
		Title:  "Feature a",
		URL:    "https://example.com/pr/11",
		State:  "OPEN",
	}}))
	if !strings.Contains(updated, "Intro") || !strings.Contains(updated, "Outro") {
		t.Fatalf("expected text around managed block to stay intact, got:\n%s", updated)
	}
}

func TestManagedStackBlockKeepsHeadingInsideManagedMarkers(t *testing.T) {
	managed := managedStackBlock("feat-a", []StackPRLine{{
		Branch: "feat-a",
		Number: 11,
		Title:  "Feature a",
		URL:    "https://example.com/pr/11",
		State:  "OPEN",
	}})
	start := strings.Index(managed, managedBlockStart)
	heading := strings.Index(managed, "## Current Stack")
	end := strings.Index(managed, managedBlockEnd)
	if !(start >= 0 && heading > start && end > heading) {
		t.Fatalf("expected heading to be inside managed markers, got:\n%s", managed)
	}
}

func TestStackPRMarker(t *testing.T) {
	if got := stackPRMarker("feat-b", "feat-b", "OPEN"); got != "👉" {
		t.Fatalf("expected current marker, got %q", got)
	}
	if got := stackPRMarker("feat-b", "feat-a", "MERGED"); got != "☑️" {
		t.Fatalf("expected merged marker, got %q", got)
	}
	if got := stackPRMarker("feat-b", "feat-a", "OPEN"); got != "⚪" {
		t.Fatalf("expected open marker, got %q", got)
	}
	if got := stackPRMarker("feat-b", "feat-a", " merged "); got != "☑️" {
		t.Fatalf("expected merged marker with surrounding spaces, got %q", got)
	}
}
