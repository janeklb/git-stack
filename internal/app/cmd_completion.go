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
		if err == nil {
			_, err = buf.WriteString("\n" + fishGitSubcommandWrapper)
		}
	case "powershell":
		if noDesc {
			err = root.GenPowerShellCompletion(&buf)
		} else {
			err = root.GenPowerShellCompletionWithDesc(&buf)
		}
		if err == nil {
			_, err = buf.WriteString("\n" + strings.ReplaceAll(powerShellGitSubcommandWrapper, "<BT>", "`"))
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

const fishGitSubcommandWrapper = `
function __git_stack_from_git_is_active
	set -l args (commandline -opc)
	test (count $args) -ge 2; and test "$args[1]" = git; and test "$args[2]" = stack
end

function __git_stack_from_git_perform_completion
	set -l args (commandline -opc)
	set -l lastArg (string escape -- (commandline -ct))
	set -l requestComp "GIT_STACK_ACTIVE_HELP=0 git-stack __complete $args[3..-1] $lastArg"
	set -l results (eval $requestComp 2> /dev/null)

	for line in $results[-1..1]
		if test (string trim -- $line) = ""
			set results $results[1..-2]
		else
			break
		end
	end

	set -l comps $results[1..-2]
	set -l directiveLine $results[-1]
	set -l flagPrefix (string match -r -- '-.*=' "$lastArg")

	for comp in $comps
		printf "%s%s\n" "$flagPrefix" "$comp"
	end

	printf "%s\n" "$directiveLine"
end

function __git_stack_from_git_perform_completion_once
	if test -n "$__git_stack_from_git_perform_completion_once_result"
		return 0
	end

	set --global __git_stack_from_git_perform_completion_once_result (__git_stack_from_git_perform_completion)
	if test -z "$__git_stack_from_git_perform_completion_once_result"
		return 1
	end

	return 0
end

function __git_stack_from_git_clear_perform_completion_once_result
	set --erase __git_stack_from_git_perform_completion_once_result
end

function __git_stack_from_git_requires_order_preservation
	__git_stack_from_git_perform_completion_once
	or return 1

	set -l directive (string sub --start 2 $__git_stack_from_git_perform_completion_once_result[-1])
	set -l shellCompDirectiveKeepOrder 32
	test (math (math --scale 0 $directive / $shellCompDirectiveKeepOrder) % 2) -ne 0
end

function __git_stack_from_git_prepare_completions
	set --erase __git_stack_from_git_comp_results

	__git_stack_from_git_perform_completion_once
	or return 1

	set -l directive (string sub --start 2 $__git_stack_from_git_perform_completion_once_result[-1])
	set --global __git_stack_from_git_comp_results $__git_stack_from_git_perform_completion_once_result[1..-2]

	set -l shellCompDirectiveError 1
	set -l shellCompDirectiveNoSpace 2
	set -l shellCompDirectiveNoFileComp 4
	set -l shellCompDirectiveFilterFileExt 8
	set -l shellCompDirectiveFilterDirs 16

	if test -z "$directive"
		set directive 0
	end

	if test (math (math --scale 0 $directive / $shellCompDirectiveError) % 2) -eq 1
		return 1
	end

	if test (math (math --scale 0 $directive / $shellCompDirectiveFilterFileExt) % 2) -eq 1
		return 1
	end
	if test (math (math --scale 0 $directive / $shellCompDirectiveFilterDirs) % 2) -eq 1
		return 1
	end

	set -l nospace (math (math --scale 0 $directive / $shellCompDirectiveNoSpace) % 2)
	set -l nofiles (math (math --scale 0 $directive / $shellCompDirectiveNoFileComp) % 2)

	if test $nospace -ne 0; or test $nofiles -eq 0
		set -l prefix (commandline -t | string escape --style=regex)
		set -l completions (string match -r -- "^$prefix.*" $__git_stack_from_git_comp_results)
		set --global __git_stack_from_git_comp_results $completions

		set -l numComps (count $__git_stack_from_git_comp_results)
		if test $numComps -eq 1; and test $nospace -ne 0
			set -l split (string split --max 1 \t $__git_stack_from_git_comp_results[1])
			set -l lastChar (string sub -s -1 -- $split)
			if not string match -r -q "[@=/:.,]" -- "$lastChar"
				set --global __git_stack_from_git_comp_results $split[1] $split[1].
			end
		end

		if test $numComps -eq 0; and test $nofiles -eq 0
			return 1
		end
	end

	return 0
end

complete -c git -n '__git_stack_from_git_is_active; and __git_stack_from_git_clear_perform_completion_once_result' > /dev/null 2>&1
complete -c git -n '__git_stack_from_git_is_active; and not __git_stack_from_git_requires_order_preservation; and __git_stack_from_git_prepare_completions' -f -a '$__git_stack_from_git_comp_results'
complete -k -c git -n '__git_stack_from_git_is_active; and __git_stack_from_git_requires_order_preservation; and __git_stack_from_git_prepare_completions' -f -a '$__git_stack_from_git_comp_results'
`

const powerShellGitSubcommandWrapper = `
[scriptblock]${__git_stackGitCompleterBlock} = {
    param(
            $WordToComplete,
            $CommandAst,
            $CursorPosition
        )

    $CommandElements = $CommandAst.CommandElements
    if ($CommandElements.Count -lt 2) {
        return
    }

    if ("$($CommandElements[1])" -ne "stack") {
        return
    }

    $Command = $CommandElements | Select-Object -Skip 1
    $Command = "git-stack $($Command | Select-Object -Skip 1)"

    $ShellCompDirectiveError=1
    $ShellCompDirectiveNoSpace=2
    $ShellCompDirectiveNoFileComp=4
    $ShellCompDirectiveFilterFileExt=8
    $ShellCompDirectiveFilterDirs=16
    $ShellCompDirectiveKeepOrder=32

    if ($Command.Length -gt ($CursorPosition - 4)) {
        $Command=$Command.Substring(0,$CursorPosition - 4)
    }

    $Program,$Arguments = $Command.Split(" ",2)
    $RequestComp="$Program __complete $Arguments"

    if ($WordToComplete -ne "" ) {
        $WordToComplete = $Arguments.Split(" ")[-1]
    }

    $IsEqualFlag = ($WordToComplete -Like "--*=*" )
    if ( $IsEqualFlag ) {
        $Flag,$WordToComplete = $WordToComplete.Split("=",2)
    }

    if ( $WordToComplete -eq "" -And ( -Not $IsEqualFlag )) {
        if ($PSVersionTable.PsVersion -lt [version]'7.2.0' -or
            ($PSVersionTable.PsVersion -lt [version]'7.3.0' -and -not [ExperimentalFeature]::IsEnabled("PSNativeCommandArgumentPassing")) -or
            (($PSVersionTable.PsVersion -ge [version]'7.3.0' -or [ExperimentalFeature]::IsEnabled("PSNativeCommandArgumentPassing")) -and
              $PSNativeCommandArgumentPassing -eq 'Legacy')) {
             $RequestComp="$RequestComp" + ' <BT>"<BT>"'
        } else {
             $RequestComp="$RequestComp" + ' ""'
        }
    }

    ${env:GIT_STACK_ACTIVE_HELP}=0
    Invoke-Expression -OutVariable out "$RequestComp" 2>&1 | Out-Null

    [int]$Directive = $Out[-1].TrimStart(':')
    if ($Directive -eq "") {
        $Directive = 0
    }

    $Out = $Out | Where-Object { $_ -ne $Out[-1] }

    if (($Directive -band $ShellCompDirectiveError) -ne 0 ) {
        return
    }

    $Longest = 0
    [Array]$Values = $Out | ForEach-Object {
        $Name, $Description = $_.Split("<BT>t",2)
        if ($Longest -lt $Name.Length) {
            $Longest = $Name.Length
        }
        if (-Not $Description) {
            $Description = " "
        }
        @{Name="$Name";Description="$Description"}
    }

    $Space = " "
    if (($Directive -band $ShellCompDirectiveNoSpace) -ne 0 ) {
        $Space = ""
    }

    if ((($Directive -band $ShellCompDirectiveFilterFileExt) -ne 0 ) -or
       (($Directive -band $ShellCompDirectiveFilterDirs) -ne 0 ))  {
        return
    }

    $Values = $Values | Where-Object {
        $_.Name -like "$WordToComplete*"
        if ( $IsEqualFlag ) {
            $_.Name = $Flag + "=" + $_.Name
        }
    }

    if (($Directive -band $ShellCompDirectiveKeepOrder) -eq 0 ) {
        $Values = $Values | Sort-Object -Property Name
    }

    if (($Directive -band $ShellCompDirectiveNoFileComp) -ne 0 ) {
        if ($Values.Length -eq 0) {
            ""
            return
        }
    }

    $Mode = (Get-PSReadLineKeyHandler | Where-Object {$_.Key -eq "Tab" }).Function

    $Values | ForEach-Object {
        $comp = $_

        switch ($Mode) {
            "Complete" {
                if ($Values.Length -eq 1) {
                    [System.Management.Automation.CompletionResult]::new($($comp.Name | __git-stack_escapeStringWithSpecialChars) + $Space, "$($comp.Name)", 'ParameterValue', "$($comp.Description)")
                } else {
                    while($comp.Name.Length -lt $Longest) {
                        $comp.Name = $comp.Name + " "
                    }

                    if ($($comp.Description) -eq " " ) {
                        $Description = ""
                    } else {
                        $Description = "  ($($comp.Description))"
                    }

                    [System.Management.Automation.CompletionResult]::new("$($comp.Name)$Description", "$($comp.Name)$Description", 'ParameterValue', "$($comp.Description)")
                }
             }

            "MenuComplete" {
                [System.Management.Automation.CompletionResult]::new($($comp.Name | __git-stack_escapeStringWithSpecialChars) + $Space, "$($comp.Name)", 'ParameterValue', "$($comp.Description)")
            }

            Default {
                [System.Management.Automation.CompletionResult]::new($($comp.Name | __git-stack_escapeStringWithSpecialChars), "$($comp.Name)", 'ParameterValue', "$($comp.Description)")
            }
        }

    }
}

Register-ArgumentCompleter -CommandName 'git' -ScriptBlock ${__git_stackGitCompleterBlock}
`
