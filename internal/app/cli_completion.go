package app

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func completeBranchRefs(includeRemote bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		branches, err := listBranchCompletionCandidates(includeRemote)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		matches := make([]string, 0, len(branches))
		for _, branch := range branches {
			if strings.HasPrefix(branch, toComplete) {
				matches = append(matches, branch)
			}
		}
		return matches, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeSingleBranchArg(includeRemote bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	branchCompletion := completeBranchRefs(includeRemote)
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return branchCompletion(cmd, args, toComplete)
	}
}

func listBranchCompletionCandidates(includeRemote bool) ([]string, error) {
	branches, err := listLocalBranches()
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(branches))
	for _, branch := range branches {
		seen[branch] = struct{}{}
	}

	if includeRemote {
		remoteBranches, err := listOriginBranches()
		if err != nil {
			return nil, err
		}
		for _, branch := range remoteBranches {
			seen[branch] = struct{}{}
		}
	}

	branches = branches[:0]
	for branch := range seen {
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	return branches, nil
}
