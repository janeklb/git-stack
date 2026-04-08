package app

import "strings"

func normalizeState(state *State) {
	if state == nil {
		return
	}

	state.Trunk = strings.TrimSpace(state.Trunk)
	state.RestackMode = strings.TrimSpace(state.RestackMode)
	state.Naming.Template = strings.TrimSpace(state.Naming.Template)
	state.Cleanup.MergeDetection = strings.TrimSpace(state.Cleanup.MergeDetection)

	if state.Branches == nil {
		state.Branches = map[string]*BranchRef{}
	}
	if state.Archived == nil {
		state.Archived = map[string]*ArchivedRef{}
	}

	for _, branch := range state.Branches {
		normalizeBranchRef(branch)
	}
	for _, branch := range state.Archived {
		normalizeArchivedRef(branch)
	}
}

func normalizeBranchRef(branch *BranchRef) {
	if branch == nil {
		return
	}
	branch.Parent = strings.TrimSpace(branch.Parent)
	branch.LineageParent = strings.TrimSpace(branch.LineageParent)
	if branch.LineageParent == "" {
		branch.LineageParent = branch.Parent
	}
	normalizePRMeta(branch.PR)
}

func normalizeArchivedRef(branch *ArchivedRef) {
	if branch == nil {
		return
	}
	branch.Parent = strings.TrimSpace(branch.Parent)
	normalizePRMeta(branch.PR)
}

func normalizePRMeta(pr *PRMeta) {
	if pr == nil {
		return
	}
	pr.URL = strings.TrimSpace(pr.URL)
	pr.Base = strings.TrimSpace(pr.Base)
}

func normalizeOperation(op *RestackOperation) {
	if op == nil {
		return
	}

	op.Type = strings.TrimSpace(op.Type)
	op.Mode = strings.TrimSpace(op.Mode)
	op.OriginalBranch = strings.TrimSpace(op.OriginalBranch)

	for i := range op.Queue {
		op.Queue[i] = strings.TrimSpace(op.Queue[i])
	}
	for branch, head := range op.OriginalHeads {
		op.OriginalHeads[branch] = strings.TrimSpace(head)
	}
	for branch, base := range op.RebaseBases {
		op.RebaseBases[branch] = strings.TrimSpace(base)
	}
}

func normalizeGHJSON(out any) {
	switch v := out.(type) {
	case *GhPR:
		normalizeGhPR(v)
	case *[]GhPR:
		for i := range *v {
			normalizeGhPR(&(*v)[i])
		}
	}
}

func normalizeGhPR(pr *GhPR) {
	if pr == nil {
		return
	}

	pr.URL = strings.TrimSpace(pr.URL)
	pr.BaseRefName = strings.TrimSpace(pr.BaseRefName)
	pr.HeadRefOID = strings.TrimSpace(pr.HeadRefOID)
	pr.Title = strings.TrimSpace(pr.Title)
	pr.State = strings.TrimSpace(pr.State)
	if pr.MergeCommit != nil {
		pr.MergeCommit.OID = strings.TrimSpace(pr.MergeCommit.OID)
	}
}
