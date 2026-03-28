package app

import (
	"fmt"
	"strings"
)

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

func composeBody(summary []string, managed string) string {
	var b strings.Builder
	b.WriteString("## Summary\n")
	for _, item := range summary {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(managed)
	b.WriteString("\n")
	return b.String()
}

func managedBlock(branch, parent string) string {
	return strings.Join([]string{
		managedBlockStart,
		fmt.Sprintf("branch: %s", branch),
		fmt.Sprintf("parent: %s", parent),
		"managed-by: stack",
		managedBlockEnd,
	}, "\n")
}

func upsertManagedBlock(body, managed string) string {
	start := strings.Index(body, managedBlockStart)
	if start >= 0 {
		return strings.TrimSpace(body[:start]) + "\n\n" + managed + "\n"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return managed + "\n"
	}
	return body + "\n\n" + managed + "\n"
}
