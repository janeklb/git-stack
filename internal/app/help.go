package app

import (
	"fmt"
	"io"
	"os"
	"regexp"
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

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func (a *App) helpFunc(cmd *cobra.Command, _ []string) {
	theme := helpTheme{useColor: helpColorEnabled(cmd.OutOrStdout())}
	writeCommandHelp(cmd.OutOrStdout(), cmd, configuredHelpWrapWidth(cmd.OutOrStdout()), theme)
}

func (a *App) writeCompactRootHelp(out io.Writer, cmd *cobra.Command) {
	theme := helpTheme{useColor: helpColorEnabled(out)}
	writeCompactRootHelp(out, cmd, configuredHelpWrapWidth(out), theme)
}

func helpColorEnabled(out io.Writer) bool {
	return stdoutIsTTY(out) && strings.TrimSpace(os.Getenv("NO_COLOR")) == ""
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

func writeCommandHelp(out io.Writer, cmd *cobra.Command, wrapWidth int, theme helpTheme) {
	writeHelpDescription(out, cmd, wrapWidth, theme)
	writeUsageSection(out, cmd, theme)
	writeAliasesSection(out, cmd, theme)
	writeCommandSection(out, cmd, wrapWidth, theme)
	writeExampleSection(out, cmd, wrapWidth, theme)
	writeFlagsSection(out, "Flags:", cmd.NonInheritedFlags(), wrapWidth, theme)
	writeFlagsSection(out, "Global Flags:", cmd.InheritedFlags(), wrapWidth, theme)
	writeFooter(out, cmd, theme)
}

func writeCompactRootHelp(out io.Writer, cmd *cobra.Command, wrapWidth int, theme helpTheme) {
	short := strings.TrimSpace(cmd.Short)
	if short != "" {
		writeWrappedText(out, theme.summary(short), wrapWidth, "")
		fmt.Fprintln(out)
		fmt.Fprintln(out)
	}
	writeUsageSection(out, cmd, theme)
	writeCommandSection(out, cmd, wrapWidth, theme)
	writeFooter(out, cmd, theme)
}

func writeHelpDescription(out io.Writer, cmd *cobra.Command, wrapWidth int, theme helpTheme) {
	short := strings.TrimSpace(cmd.Short)
	long := strings.TrimSpace(cmd.Long)
	if short == "" && long == "" {
		return
	}
	if short != "" {
		writeWrappedText(out, theme.summary(short), wrapWidth, "")
		if long != "" && long != short {
			fmt.Fprintln(out)
		}
	}
	if long != "" && long != short {
		writeWrappedText(out, long, wrapWidth, "")
	}
	fmt.Fprintln(out)
}

func writeUsageSection(out io.Writer, cmd *cobra.Command, theme helpTheme) {
	fmt.Fprintln(out, theme.header("Usage:"))
	fmt.Fprintf(out, "  %s\n", cmd.UseLine())
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(out, "  %s [command]\n", cmd.CommandPath())
	}
}

func writeAliasesSection(out io.Writer, cmd *cobra.Command, theme helpTheme) {
	if len(cmd.Aliases) == 0 {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, theme.header("Aliases:"))
	fmt.Fprintf(out, "  %s\n", strings.Join(cmd.Aliases, ", "))
}

func writeCommandSection(out io.Writer, cmd *cobra.Command, wrapWidth int, theme helpTheme) {
	children := availableCommands(cmd)
	if len(children) == 0 {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, theme.header("Available Commands:"))
	nameWidth := 0
	for _, child := range children {
		if n := runeWidth(child.Name()); n > nameWidth {
			nameWidth = n
		}
	}
	for _, child := range children {
		writeAlignedEntry(out, theme.command(child.Name()), child.Short, nameWidth, wrapWidth)
	}
}

func writeExampleSection(out io.Writer, cmd *cobra.Command, wrapWidth int, theme helpTheme) {
	example := strings.Trim(cmd.Example, "\n")
	if example == "" {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, theme.header("Examples:"))
	for _, line := range strings.Split(example, "\n") {
		writeWrappedText(out, theme.example(strings.TrimRight(line, " \t")), wrapWidth, "")
	}
}

func writeFlagsSection(out io.Writer, title string, flags *flag.FlagSet, wrapWidth int, theme helpTheme) {
	if flags == nil || !flags.HasAvailableFlags() {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, theme.header(title))
	writeFlagUsages(out, flags, wrapWidth, theme)
}

func writeFooter(out io.Writer, cmd *cobra.Command, theme helpTheme) {
	if cmd.HasSubCommands() {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "%s\n", theme.note(fmt.Sprintf("Use \"%s [command] --help\" for more information about a command.", cmd.CommandPath())))
	}
}

func writeFlagUsages(out io.Writer, flags *flag.FlagSet, wrapWidth int, theme helpTheme) {
	entries := collectFlagEntries(flags, theme)
	if len(entries) == 0 {
		return
	}
	nameWidth := 0
	for _, entry := range entries {
		if entry.plainWidth > nameWidth {
			nameWidth = entry.plainWidth
		}
	}
	for _, entry := range entries {
		writeAlignedEntry(out, entry.styledName, entry.usage, nameWidth, wrapWidth)
	}
}

type helpFlagEntry struct {
	styledName string
	plainWidth int
	usage      string
}

func collectFlagEntries(flags *flag.FlagSet, theme helpTheme) []helpFlagEntry {
	entries := []helpFlagEntry{}
	flags.VisitAll(func(f *flag.Flag) {
		if f.Hidden {
			return
		}
		plainName, styledName := formatFlagName(f, theme)
		entries = append(entries, helpFlagEntry{
			styledName: styledName,
			plainWidth: runeWidth(plainName),
			usage:      strings.TrimSpace(f.Usage),
		})
	})
	return entries
}

func formatFlagName(f *flag.Flag, theme helpTheme) (string, string) {
	partsPlain := []string{}
	partsStyled := []string{}
	if f.Shorthand != "" && f.ShorthandDeprecated == "" {
		shorthand := "-" + f.Shorthand
		partsPlain = append(partsPlain, shorthand)
		partsStyled = append(partsStyled, theme.flagName(shorthand))
	}
	long := "--" + f.Name
	partsPlain = append(partsPlain, long)
	partsStyled = append(partsStyled, theme.flagName(long))
	plainName := strings.Join(partsPlain, ", ")
	styledName := strings.Join(partsStyled, ", ")
	if valueName := flagValueName(f); valueName != "" {
		plainName += " " + valueName
		styledName += " " + theme.flagValue(valueName)
	}
	return plainName, styledName
}

func flagValueName(f *flag.Flag) string {
	if f.Value == nil {
		return ""
	}
	if f.Value.Type() == "bool" {
		return ""
	}
	name := strings.TrimSpace(f.Value.Type())
	if name == "" {
		name = "value"
	}
	return name
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
	prefix := "  " + name + strings.Repeat(" ", max(0, nameWidth-visibleRuneWidth(name))) + "  "
	if wrapWidth <= 0 {
		fmt.Fprintf(out, "%s%s\n", prefix, description)
		return
	}
	continuation := strings.Repeat(" ", visibleRuneWidth(prefix))
	lines := wrapLine(description, wrapWidth-visibleRuneWidth(prefix))
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

func visibleRuneWidth(text string) int {
	return runeWidth(stripANSI(text))
}

func stripANSI(text string) string {
	return ansiEscapePattern.ReplaceAllString(text, "")
}

type helpTheme struct {
	useColor bool
}

func (t helpTheme) header(text string) string {
	return t.wrap(text, "1;37")
}

func (t helpTheme) summary(text string) string {
	return t.wrap(text, "1")
}

func (t helpTheme) command(text string) string {
	return t.wrap(text, "1;36")
}

func (t helpTheme) flagName(text string) string {
	return t.wrap(text, "33")
}

func (t helpTheme) flagValue(text string) string {
	return t.wrap(text, "36")
}

func (t helpTheme) example(text string) string {
	return t.wrap(text, "2")
}

func (t helpTheme) note(text string) string {
	return t.wrap(text, "90")
}

func (t helpTheme) wrap(text, code string) string {
	if !t.useColor {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
