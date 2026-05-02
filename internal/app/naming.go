package app

import (
	"fmt"
	"regexp"
	"strings"
)

func slugify(input string) string {
	s := strings.TrimSpace(input)
	re := regexp.MustCompile(`[^A-Za-z0-9._/-]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-./")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

func applyTemplate(template, slug string, index int, usePrefix bool) string {
	if strings.TrimSpace(template) == "" {
		template = "{slug}"
	}
	name := strings.ReplaceAll(template, "{slug}", slug)
	name = strings.ReplaceAll(name, "{n}", fmt.Sprintf("%03d", index))
	if usePrefix && !strings.Contains(template, "{n}") {
		name = fmt.Sprintf("%03d-%s", index, name)
	}
	return name
}
