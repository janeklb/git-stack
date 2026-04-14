package app

import (
	"strings"
	"testing"
)

func TestUpsertManagedBlockAppendsAndReplaces(t *testing.T) {
	t.Parallel()

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
	if !strings.Contains(body, "## Stacked PRs") {
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
	t.Parallel()

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
	t.Parallel()

	managed := managedStackBlock("feat-a", []StackPRLine{{
		Branch: "feat-a",
		Number: 11,
		Title:  "Feature a",
		URL:    "https://example.com/pr/11",
		State:  "OPEN",
	}})
	start := strings.Index(managed, managedBlockStart)
	heading := strings.Index(managed, "## Stacked PRs")
	end := strings.Index(managed, managedBlockEnd)
	if !(start >= 0 && heading > start && end > heading) {
		t.Fatalf("expected heading to be inside managed markers, got:\n%s", managed)
	}
}

func TestComposeBodyUsesDefaultSummaryAndManagedSection(t *testing.T) {
	t.Parallel()

	managed := managedStackBlock("feat-a", []StackPRLine{{
		Branch: "feat-a",
		Number: 11,
		Title:  "Feature a",
		URL:    "https://example.com/pr/11",
		State:  "OPEN",
	}})
	body := composeBody([]string{"Added validation", "Refined output format"}, managed, "")
	if !strings.Contains(body, "## Summary") {
		t.Fatalf("expected body to include summary heading, got:\n%s", body)
	}
	if !strings.Contains(body, "- Added validation") || !strings.Contains(body, "- Refined output format") {
		t.Fatalf("expected body to include summary bullets, got:\n%s", body)
	}
	if !strings.Contains(body, "## Stacked PRs") {
		t.Fatalf("expected body to include managed stacked PR section, got:\n%s", body)
	}
	if !strings.HasSuffix(body, "\n") {
		t.Fatalf("expected body to end with newline, got %q", body)
	}
	if strings.Index(body, "## Summary") > strings.Index(body, "## Stacked PRs") {
		t.Fatalf("expected summary before stacked PRs, got:\n%s", body)
	}
}

func TestComposeBodyUsesCustomTemplatePlaceholders(t *testing.T) {
	t.Parallel()

	managed := managedStackBlock("feat-a", []StackPRLine{{
		Branch: "feat-a",
		Number: 11,
		Title:  "Feature a",
		URL:    "https://example.com/pr/11",
		State:  "OPEN",
	}})
	template := strings.Join([]string{
		"Before",
		prSummaryPlaceholder,
		"Between",
		stackedPRsPlaceholder,
		"After",
	}, "\n\n")
	body := composeBody([]string{"Added validation"}, managed, template)
	if !strings.Contains(body, "Before") || !strings.Contains(body, "Between") || !strings.Contains(body, "After") {
		t.Fatalf("expected custom template text to be preserved, got:\n%s", body)
	}
	if !strings.Contains(body, "## Summary") || !strings.Contains(body, "- Added validation") {
		t.Fatalf("expected summary placeholder to be replaced, got:\n%s", body)
	}
	if !strings.Contains(body, "## Stacked PRs") {
		t.Fatalf("expected stacked PR placeholder to be replaced, got:\n%s", body)
	}
}

func TestComposeBodyFallsBackWhenTemplateOmitsPlaceholders(t *testing.T) {
	t.Parallel()

	managed := managedStackBlock("feat-a", []StackPRLine{{
		Branch: "feat-a",
		Number: 11,
		Title:  "Feature a",
		URL:    "https://example.com/pr/11",
		State:  "OPEN",
	}})
	body := composeBody([]string{"Added validation"}, managed, "## Details\n\nCustom body")
	if !strings.Contains(body, "## Summary") || !strings.Contains(body, "## Details") {
		t.Fatalf("expected summary prepended ahead of custom template, got:\n%s", body)
	}
	if !strings.Contains(body, "## Stacked PRs") {
		t.Fatalf("expected managed section appended when placeholder missing, got:\n%s", body)
	}
	if strings.Index(body, "## Summary") > strings.Index(body, "## Details") {
		t.Fatalf("expected summary before custom template, got:\n%s", body)
	}
}

func TestStackPRMarker(t *testing.T) {
	t.Parallel()

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

func TestShouldUseDraftPR(t *testing.T) {
	t.Parallel()

	if shouldUseDraftPR("main", "main") {
		t.Fatal("expected trunk-based PR to be ready for review")
	}
	if !shouldUseDraftPR("main", "feat-parent") {
		t.Fatal("expected stacked PR to stay draft")
	}
}
