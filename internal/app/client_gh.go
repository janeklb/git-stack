package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
)

func (r *repository) FindByHead(branch string) (*GhPR, error) {
	return r.ghFindSingleByHead(branch, "open")
}

func (r *repository) FindMergedByHead(branch string) (*GhPR, error) {
	return r.ghFindSingleByHead(branch, "merged")
}

func (r *repository) ghFindSingleByHead(branch string, state string) (*GhPR, error) {
	var prs []GhPR
	if err := r.ghJSON(&prs, "pr", "list", "--head", branch, "--state", state, "--json", "number,url,body,baseRefName,headRefOid,title,isDraft,state,mergeCommit", "--limit", "2"); err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, nil
	}
	if len(prs) > 1 {
		return nil, fmt.Errorf("ambiguous %s PR lookup for head %s", state, branch)
	}
	return &prs[0], nil
}

func (r *repository) View(number int) (*GhPR, error) {
	var pr GhPR
	if err := r.ghJSON(&pr, "pr", "view", strconv.Itoa(number), "--json", "number,url,body,baseRefName,headRefOid,title,isDraft,state,mergeCommit"); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (r *repository) ghCreate(branch, parent, title, body string, draft bool) (int, string, error) {
	bodyFile, cleanup, err := writeTempBody(body)
	if err != nil {
		return 0, "", err
	}
	defer cleanup()

	args := []string{"pr", "create", "--base", parent, "--head", branch, "--title", title, "--body-file", bodyFile}
	if draft {
		args = append(args, "--draft")
	}
	if err := r.ghRun(args...); err != nil {
		return 0, "", err
	}
	pr, err := r.FindByHead(branch)
	if err != nil {
		return 0, "", err
	}
	if pr == nil {
		return 0, "", errors.New("created PR but failed to query it")
	}
	return pr.Number, pr.URL, nil
}

func (r *repository) ghEdit(number int, base, body string) error {
	bodyFile, cleanup, err := writeTempBody(body)
	if err != nil {
		return err
	}
	defer cleanup()
	return r.ghRun("pr", "edit", strconv.Itoa(number), "--base", base, "--body-file", bodyFile)
}

func (r *repository) ghSyncDraftState(number int, wantDraft bool, current *GhPR) error {
	if current != nil && current.IsDraft == wantDraft {
		return nil
	}
	args := []string{"pr", "ready", strconv.Itoa(number)}
	if wantDraft {
		args = append(args, "--undo")
	}
	return r.ghRun(args...)
}

func writeTempBody(body string) (string, func(), error) {
	f, err := os.CreateTemp("", "stack-pr-body-*.md")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	return f.Name(), cleanup, nil
}

func (r *repository) ghJSON(out any, args ...string) error {
	raw, err := r.ghOutput(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return err
	}
	normalizeGHJSON(out)
	return nil
}

func (r *repository) ghRun(args ...string) error {
	_, err := r.run("gh", args, commandRunOptions{streamOutput: true, boxMode: commandBoxOnFailure})
	return err
}

func (r *repository) ghOutput(args ...string) (string, error) {
	result, err := r.run("gh", args, commandRunOptions{streamOutput: false, boxMode: commandBoxOnFailure})
	return result.stdout, err
}
