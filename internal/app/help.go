package app

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

const (
	preferredHelpWrapWidth = 88
	minimumHelpWrapWidth   = 100
)

func (a *App) helpFunc(cmd *cobra.Command, _ []string) {
	writeCommandHelp(cmd.OutOrStdout(), cmd, configuredHelpWrapWidth(cmd.OutOrStdout()))
}

func configuredHelpWrapWidth(out io.Writer) int {
	if !stdoutIsTTY(out) {
		return 0
	}
	if width := helpWrapWidthForTerminalWidth(helpTerminalWidth(out)); width > 0 {
		return width
	}
	return 0
}

func helpTerminalWidth(out io.Writer) int {
	if value := strings.TrimSpace(os.Getenv("COLUMNS")); value != "" {
		if width, err := strconv.Atoi(value); err == nil && width > 0 {
			return width
		}
	}
	file, ok := out.(*os.File)
	if !ok {
		return 0
	}
	width, err := terminalWidth(file)
	if err != nil {
		return 0
	}
	return width
}

func helpWrapWidthForTerminalWidth(width int) int {
	if width < minimumHelpWrapWidth {
		return 0
	}
	if width < preferredHelpWrapWidth {
		return width
	}
	return preferredHelpWrapWidth
}

func writeCommandHelp(out io.Writer, cmd *cobra.Command, wrapWidth int) {
	writeHelpDescription(out, cmd, wrapWidth)
	writeUsageSection(out, cmd)
	writeAliasesSection(out, cmd)
	writeCommandSection(out, cmd, wrapWidth)
	writeExampleSection(out, cmd, wrapWidth)
	writeFlagsSection(out, "Flags:", cmd.NonInheritedFlags(), wrapWidth)
	writeFlagsSection(out, "Global Flags:", cmd.InheritedFlags(), wrapWidth)
	writeFooter(out, cmd)
}

func writeHelpDescription(out io.Writer, cmd *cobra.Command, wrapWidth int) {
	text := strings.TrimSpace(cmd.Long)
	if text == "" {
		text = strings.TrimSpace(cmd.Short)
	}
	if text == "" {
		return
	}
	writeWrappedText(out, text, wrapWidth, "")
	fmt.Fprintln(out)
	fmt.Fprintln(out)
}

func writeUsageSection(out io.Writer, cmd *cobra.Command) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintf(out, "  %s\n", cmd.UseLine())
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(out, "  %s [command]\n", cmd.CommandPath())
	}
}

func writeAliasesSection(out io.Writer, cmd *cobra.Command) {
	if len(cmd.Aliases) == 0 {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Aliases:")
	fmt.Fprintf(out, "  %s\n", strings.Join(cmd.Aliases, ", "))
}

func writeCommandSection(out io.Writer, cmd *cobra.Command, wrapWidth int) {
	children := availableCommands(cmd)
	if len(children) == 0 {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Available Commands:")
	nameWidth := 0
	for _, child := range children {
		if n := runeWidth(child.Name()); n > nameWidth {
			nameWidth = n
		}
	}
	for _, child := range children {
		writeAlignedEntry(out, child.Name(), child.Short, nameWidth, wrapWidth)
	}
}

func writeExampleSection(out io.Writer, cmd *cobra.Command, wrapWidth int) {
	example := strings.Trim(cmd.Example, "\n")
	if example == "" {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Examples:")
	for _, line := range strings.Split(example, "\n") {
		writeWrappedText(out, strings.TrimRight(line, " \t"), wrapWidth, "")
	}
}

func writeFlagsSection(out io.Writer, title string, flags *flag.FlagSet, wrapWidth int) {
	if flags == nil || !flags.HasAvailableFlags() {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, title)
	if wrapWidth > 0 {
		fmt.Fprint(out, flags.FlagUsagesWrapped(wrapWidth))
		return
	}
	fmt.Fprint(out, flags.FlagUsages())
}

func writeFooter(out io.Writer, cmd *cobra.Command) {
	if cmd.HasSubCommands() {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Use \"%s [command] --help\" for more information about a command.\n", cmd.CommandPath())
	}
}

func availableCommands(cmd *cobra.Command) []*cobra.Command {
	children := []*cobra.Command{}
	for _, child := range cmd.Commands() {
		if child.Name() != "help" && !child.IsAvailableCommand() && !child.IsAdditionalHelpTopicCommand() {
			continue
		}
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name() < children[j].Name()
	})
	return children
}

func writeAlignedEntry(out io.Writer, name, description string, nameWidth int, wrapWidth int) {
	if strings.TrimSpace(description) == "" {
		fmt.Fprintf(out, "  %s\n", name)
		return
	}
	prefix := fmt.Sprintf("  %-*s  ", nameWidth, name)
	if wrapWidth <= 0 {
		fmt.Fprintf(out, "%s%s\n", prefix, description)
		return
	}
	continuation := strings.Repeat(" ", runeWidth(prefix))
	lines := wrapLine(description, wrapWidth-runeWidth(prefix))
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(out, "%s%s\n", prefix, line)
			continue
		}
		fmt.Fprintf(out, "%s%s\n", continuation, line)
	}
}

func writeWrappedText(out io.Writer, text string, wrapWidth int, indent string) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmedRight) == "" {
			fmt.Fprintln(out)
			continue
		}
		leading := leadingWhitespace(trimmedRight)
		content := strings.TrimSpace(trimmedRight)
		prefix := indent + leading
		if wrapWidth <= 0 {
			fmt.Fprintf(out, "%s%s\n", prefix, content)
			continue
		}
		available := wrapWidth - runeWidth(prefix)
		if available <= 0 {
			available = wrapWidth
		}
		wrapped := wrapLine(content, available)
		for _, part := range wrapped {
			fmt.Fprintf(out, "%s%s\n", prefix, part)
		}
		if len(wrapped) == 0 {
			fmt.Fprintln(out, prefix)
		}
	}
}

func wrapLine(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if width <= 0 || runeWidth(text) <= width {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	lines := []string{}
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if runeWidth(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

func leadingWhitespace(text string) string {
	count := 0
	for count < len(text) {
		if text[count] != ' ' && text[count] != '\t' {
			break
		}
		count++
	}
	return text[:count]
}

func runeWidth(text string) int {
	return utf8.RuneCountInString(text)
}
