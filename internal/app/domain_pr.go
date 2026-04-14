package app

import (
	"fmt"
	"strings"
)

type StackPRLine struct {
	Branch string
	Number int
	Title  string
	URL    string
	State  string
}

func branchSummary(parent, branch string) (string, []string, error) {
	latestTitle, err := gitOutput("log", "-1", "--format=%s", branch)
	if err != nil {
		return "", nil, err
	}
	latestTitle = strings.TrimSpace(latestTitle)
	if latestTitle == "" {
		latestTitle = branch
	}
	logs, err := gitOutput("log", "--reverse", "--format=%s", fmt.Sprintf("%s..%s", parent, branch))
	if err != nil {
		return latestTitle, nil, nil
	}
	lines := []string{}
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		lines = []string{latestTitle}
	}
	return latestTitle, lines, nil
}

func composeBody(summary []string, managed, template string) string {
	summarySection := composeSummarySection(summary)
	managed = strings.TrimSpace(managed)
	template = strings.TrimSpace(template)
	if template == "" {
		return stitchBody(summarySection, managed, "")
	}

	body := template
	usedSummaryPlaceholder := strings.Contains(body, prSummaryPlaceholder)
	if usedSummaryPlaceholder {
		body = strings.ReplaceAll(body, prSummaryPlaceholder, summarySection)
	}
	if strings.Contains(body, stackedPRsPlaceholder) {
		body = strings.ReplaceAll(body, stackedPRsPlaceholder, managed)
	} else if managed != "" {
		body = stitchBody(body, managed, "")
	}
	if !usedSummaryPlaceholder {
		body = stitchBody(summarySection, strings.TrimSpace(body), "")
	}
	return strings.TrimSpace(body) + "\n"
}

func composeSummarySection(summary []string) string {
	var b strings.Builder
	b.WriteString("## Summary\n\n")
	for _, item := range summary {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func managedStackBlock(currentBranch string, lines []StackPRLine) string {
	var b strings.Builder
	b.WriteString(managedBlockStart)
	b.WriteString("\n")
	b.WriteString("## Stacked PRs\n")
	for _, line := range lines {
		if line.Number <= 0 || strings.TrimSpace(line.URL) == "" {
			continue
		}
		marker := stackPRMarker(currentBranch, line.Branch, line.State)
		title := strings.TrimSpace(line.Title)
		if title == "" {
			title = line.Branch
		}
		b.WriteString(fmt.Sprintf("- %s [#%d %s](%s)\n", marker, line.Number, title, line.URL))
	}
	b.WriteString("\n<p align=right><sub>Legend: 👉 current PR, ⚪ open, ☑️ merged</sub></p>\n")
	b.WriteString(managedBlockEnd)
	return b.String()
}

func upsertManagedBlock(body, managed string) string {
	start := strings.Index(body, managedBlockStart)
	if start >= 0 {
		relEnd := strings.Index(body[start:], managedBlockEnd)
		if relEnd >= 0 {
			end := start + relEnd + len(managedBlockEnd)
			before := strings.TrimSpace(body[:start])
			after := strings.TrimSpace(body[end:])
			return stitchBody(before, managed, after)
		}
		before := strings.TrimSpace(body[:start])
		return stitchBody(before, managed, "")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return managed + "\n"
	}
	return body + "\n\n" + managed + "\n"
}

func stackPRMarker(currentBranch, branch, state string) string {
	if currentBranch == branch {
		return "👉"
	}
	if strings.EqualFold(strings.TrimSpace(state), "merged") {
		return "☑️"
	}
	return "⚪"
}

func shouldUseDraftPR(trunk, base string) bool {
	return base != "" && base != trunk
}

func stitchBody(before, managed, after string) string {
	if before == "" && after == "" {
		return managed + "\n"
	}
	if before == "" {
		return managed + "\n\n" + after + "\n"
	}
	if after == "" {
		return before + "\n\n" + managed + "\n"
	}
	return before + "\n\n" + managed + "\n\n" + after + "\n"
}
