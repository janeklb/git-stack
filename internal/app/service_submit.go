package app

import "strings"

func ensurePR(branch, parent string, existing *PRMeta) (*PRMeta, error) {
	latestTitle, summary, err := branchSummary(parent, branch)
	if err != nil {
		return nil, err
	}
	managed := managedBlock(branch, parent)
	defaultBody := composeBody(summary, managed)

	if existing != nil && existing.Number > 0 {
		pr, err := ghView(existing.Number)
		if err == nil && strings.EqualFold(pr.State, "OPEN") {
			body := upsertManagedBlock(pr.Body, managed)
			if err := ghEdit(existing.Number, parent, body); err != nil {
				return nil, err
			}
			return &PRMeta{Number: existing.Number, URL: pr.URL, Base: parent, Updated: true}, nil
		}
	}

	if open, err := ghFindByHead(branch); err == nil && open != nil {
		body := upsertManagedBlock(open.Body, managed)
		if err := ghEdit(open.Number, parent, body); err != nil {
			return nil, err
		}
		return &PRMeta{Number: open.Number, URL: open.URL, Base: parent, Updated: true}, nil
	}

	number, url, err := ghCreate(branch, parent, latestTitle, defaultBody)
	if err != nil {
		return nil, err
	}
	return &PRMeta{Number: number, URL: url, Base: parent, Updated: false}, nil
}
