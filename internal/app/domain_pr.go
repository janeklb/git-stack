package app

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed default_pr_body.md.tmpl
var defaultPRBodyTemplate string

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

func composeBody(summary []string, managed, templateText string, hasCustomTemplate bool) (string, error) {
	if !hasCustomTemplate {
		templateText = defaultPRBodyTemplate
	}

	tmpl, err := template.New("pr_body").Option("missingkey=error").Parse(templateText)
	if err != nil {
		return "", err
	}
	data := map[string]any{
		"commits":           buildPRTemplateCommits(summary),
		"stackedPRsSection": managed,
	}
	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return "", err
	}
	return body.String(), nil
}

func buildPRTemplateCommits(summary []string) []string {
	commits := make([]string, 0, len(summary))
	for _, item := range summary {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		commits = append(commits, item)
	}
	return commits
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
	return body
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
