package app

import (
	"bytes"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

const zshCompletionFooter = `# don't run the completion function when being source-ed or eval-ed
if [ "$funcstack[1]" = "_git-stack" ]; then
	_git-stack
fi
`

func (a *App) addCompletionCmd(root *cobra.Command) {
	const noDescFlag = "no-descriptions"
	const noDescFlagShort = "no-desc"

	completionCmd := &cobra.Command{
		Use:                   "completion",
		Short:                 "Generate the autocompletion script for the specified shell",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		ValidArgsFunction:     cobra.NoFileCompletions,
	}

	var bashNoDesc bool
	bashCmd := &cobra.Command{
		Use:               "bash",
		Short:             "Generate the autocompletion script for bash",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.writeCompletionScript(cmd.OutOrStdout(), root, "bash", bashNoDesc)
		},
	}
	bashCmd.Flags().BoolVar(&bashNoDesc, noDescFlag, false, "disable completion descriptions")
	bashCmd.Flags().BoolVar(&bashNoDesc, noDescFlagShort, false, "disable completion descriptions")

	var zshNoDesc bool
	zshCmd := &cobra.Command{
		Use:               "zsh",
		Short:             "Generate the autocompletion script for zsh",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.writeCompletionScript(cmd.OutOrStdout(), root, "zsh", zshNoDesc)
		},
	}
	zshCmd.Flags().BoolVar(&zshNoDesc, noDescFlag, false, "disable completion descriptions")
	zshCmd.Flags().BoolVar(&zshNoDesc, noDescFlagShort, false, "disable completion descriptions")

	var fishNoDesc bool
	fishCmd := &cobra.Command{
		Use:               "fish",
		Short:             "Generate the autocompletion script for fish",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.writeCompletionScript(cmd.OutOrStdout(), root, "fish", fishNoDesc)
		},
	}
	fishCmd.Flags().BoolVar(&fishNoDesc, noDescFlag, false, "disable completion descriptions")
	fishCmd.Flags().BoolVar(&fishNoDesc, noDescFlagShort, false, "disable completion descriptions")

	var powerShellNoDesc bool
	powerShellCmd := &cobra.Command{
		Use:               "powershell",
		Short:             "Generate the autocompletion script for powershell",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.writeCompletionScript(cmd.OutOrStdout(), root, "powershell", powerShellNoDesc)
		},
	}
	powerShellCmd.Flags().BoolVar(&powerShellNoDesc, noDescFlag, false, "disable completion descriptions")
	powerShellCmd.Flags().BoolVar(&powerShellNoDesc, noDescFlagShort, false, "disable completion descriptions")

	completionCmd.AddCommand(bashCmd, zshCmd, fishCmd, powerShellCmd)
	root.AddCommand(completionCmd)
}

func (a *App) writeCompletionScript(out io.Writer, root *cobra.Command, shell string, noDesc bool) error {
	var buf bytes.Buffer
	var err error

	switch shell {
	case "bash":
		err = root.GenBashCompletionV2(&buf, !noDesc)
		if err == nil {
			_, err = buf.WriteString("\n" + bashGitSubcommandWrapper)
		}
	case "zsh":
		if noDesc {
			err = root.GenZshCompletionNoDesc(&buf)
		} else {
			err = root.GenZshCompletion(&buf)
		}
		if err == nil {
			script := strings.Replace(buf.String(), "_git-stack()", "__git_stack_zsh()", 1)
			script = strings.Replace(script, zshCompletionFooter, "", 1)
			buf.Reset()
			_, err = buf.WriteString(script + "\n" + zshGitSubcommandWrapper)
		}
	case "fish":
		err = root.GenFishCompletion(&buf, !noDesc)
	case "powershell":
		if noDesc {
			err = root.GenPowerShellCompletion(&buf)
		} else {
			err = root.GenPowerShellCompletionWithDesc(&buf)
		}
	default:
		return nil
	}
	if err != nil {
		return err
	}
	_, err = out.Write(buf.Bytes())
	return err
}

const bashGitSubcommandWrapper = `
_git_stack()
{
	local git_stack_words=("${words[@]}")
	local git_stack_cword=$cword
	local git_stack_cur=$cur
	local git_stack_prev=$prev

	words=("git-stack" "${git_stack_words[@]:2}")
	cword=$((git_stack_cword - 1))
	cur=$git_stack_cur
	prev=$git_stack_prev

	local out directive
	__git-stack_get_completion_results
	__git-stack_process_completion_results

	words=("${git_stack_words[@]}")
	cword=$git_stack_cword
	cur=$git_stack_cur
	prev=$git_stack_prev
}
`

const zshGitSubcommandWrapper = `
_git-stack()
{
	local git_stack_words git_stack_current
	git_stack_words=("${words[@]}")
	git_stack_current=$CURRENT

	if [[ ${words[1]} == stack ]]; then
		words=("git-stack" "${words[@]:2}")
		(( CURRENT-- ))
	fi

	__git_stack_zsh

	words=("${git_stack_words[@]}")
	CURRENT=$git_stack_current
}

# don't run the completion function when being source-ed or eval-ed
if [ "$funcstack[1]" = "_git-stack" ]; then
	_git-stack
fi
`
