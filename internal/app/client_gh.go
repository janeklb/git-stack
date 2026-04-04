package app

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
)

func ghFindByHead(branch string) (*GhPR, error) {
	var prs []GhPR
	if err := ghJSON(&prs, "pr", "list", "--head", branch, "--state", "open", "--json", "number,url,body,baseRefName,headRefOid,title,state,mergeCommit", "--limit", "1"); err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, nil
	}
	return &prs[0], nil
}

func ghFindMergedByHead(branch string) (*GhPR, error) {
	var prs []GhPR
	if err := ghJSON(&prs, "pr", "list", "--head", branch, "--state", "merged", "--json", "number,url,baseRefName,headRefOid,state,mergeCommit", "--limit", "1"); err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, nil
	}
	return &prs[0], nil
}

func ghView(number int) (*GhPR, error) {
	var pr GhPR
	if err := ghJSON(&pr, "pr", "view", strconv.Itoa(number), "--json", "number,url,body,baseRefName,headRefOid,title,state,mergeCommit"); err != nil {
		return nil, err
	}
	return &pr, nil
}

func ghCreate(branch, parent, title, body string) (int, string, error) {
	bodyFile, cleanup, err := writeTempBody(body)
	if err != nil {
		return 0, "", err
	}
	defer cleanup()

	if err := ghRun("pr", "create", "--base", parent, "--head", branch, "--title", title, "--body-file", bodyFile); err != nil {
		return 0, "", err
	}
	pr, err := ghFindByHead(branch)
	if err != nil {
		return 0, "", err
	}
	if pr == nil {
		return 0, "", errors.New("created PR but failed to query it")
	}
	return pr.Number, pr.URL, nil
}

func ghEdit(number int, base, body string) error {
	bodyFile, cleanup, err := writeTempBody(body)
	if err != nil {
		return err
	}
	defer cleanup()
	return ghRun("pr", "edit", strconv.Itoa(number), "--base", base, "--body-file", bodyFile)
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

func ghJSON(out any, args ...string) error {
	raw, err := ghOutput(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return err
	}
	return nil
}

func ghRun(args ...string) error {
	if _, err := runCommand("gh", args, commandRunOptions{streamOutput: true, boxMode: commandBoxOnFailure}); err != nil {
		return errors.New("gh " + strings.Join(args, " ") + ": " + err.Error())
	}
	return nil
}

func ghOutput(args ...string) (string, error) {
	result, err := runCommand("gh", args, commandRunOptions{streamOutput: false, boxMode: commandBoxOnFailure})
	if err != nil {
		msg := strings.TrimSpace(result.stderr)
		if msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return result.stdout, nil
}
