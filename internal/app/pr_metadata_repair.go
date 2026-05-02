package app

type trackedPRHeadLookupClient interface {
	FindByHead(branch string) (*GhPR, error)
	FindMergedByHead(branch string) (*GhPR, error)
}

func repairTrackedPRMetadata(state *State, branch string, gh trackedPRHeadLookupClient) (*GhPR, bool, error) {
	meta := state.Branches[branch]
	if meta == nil {
		return nil, false, nil
	}
	if meta.PR != nil && meta.PR.Number > 0 {
		return nil, false, nil
	}

	pr, err := gh.FindByHead(branch)
	if err != nil {
		return nil, false, err
	}
	if pr != nil {
		meta.PR = trackedPRMetaFromPR(state, branch, pr, true)
		return pr, true, nil
	}

	pr, err = gh.FindMergedByHead(branch)
	if err != nil {
		return nil, false, err
	}
	if pr == nil {
		return nil, false, nil
	}
	meta.PR = trackedPRMetaFromPR(state, branch, pr, true)
	return pr, true, nil
}

func trackedPRMetaFromPR(state *State, branch string, pr *GhPR, updated bool) *PRMeta {
	base := state.Trunk
	if meta := state.Branches[branch]; meta != nil {
		if meta.Parent != "" {
			base = meta.Parent
		}
		if meta.PR != nil && meta.PR.Base != "" {
			base = meta.PR.Base
		}
	}
	if pr != nil && pr.BaseRefName != "" {
		base = pr.BaseRefName
	}

	meta := &PRMeta{Base: base, Updated: updated}
	if pr != nil {
		meta.Number = pr.Number
		meta.URL = pr.URL
	}
	return meta
}
