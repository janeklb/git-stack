package app

import (
	"io"
	"os"
)

const (
	stateVersion              = 1
	managedBlockStart         = "<!-- stack:managed:start -->"
	managedBlockEnd           = "<!-- stack:managed:end -->"
	defaultRestackMode        = "rebase"
	cleanMergeDetectionStrict = "strict"
)

type App struct {
	in     io.Reader
	stdout io.Writer
	stderr io.Writer
}

type State struct {
	Version     int                     `json:"version"`
	Trunk       string                  `json:"trunk"`
	RestackMode string                  `json:"restackMode"`
	Naming      NamingConfig            `json:"naming"`
	Clean       CleanConfig             `json:"clean,omitempty"`
	Branches    map[string]*BranchRef   `json:"branches"`
	Archived    map[string]*ArchivedRef `json:"archived,omitempty"`
}

type CleanConfig struct {
	MergeDetection string `json:"mergeDetection,omitempty"`
}

type NamingConfig struct {
	Template    string `json:"template"`
	PrefixIndex bool   `json:"prefixIndex"`
	NextIndex   int    `json:"nextIndex"`
}

type BranchRef struct {
	Parent        string  `json:"parent"`
	LineageParent string  `json:"lineageParent,omitempty"`
	PR            *PRMeta `json:"pr,omitempty"`
}

type ArchivedRef struct {
	Parent string  `json:"parent"`
	PR     *PRMeta `json:"pr,omitempty"`
}

type PRMeta struct {
	Number  int    `json:"number"`
	URL     string `json:"url"`
	Base    string `json:"base"`
	Updated bool   `json:"updated,omitempty"`
}

type RestackOperation struct {
	Type           string            `json:"type"`
	Mode           string            `json:"mode"`
	OriginalBranch string            `json:"originalBranch"`
	Queue          []string          `json:"queue"`
	Index          int               `json:"index"`
	OriginalHeads  map[string]string `json:"originalHeads,omitempty"`
	RebaseBases    map[string]string `json:"rebaseBases,omitempty"`
}

type GhPR struct {
	Number      int       `json:"number"`
	URL         string    `json:"url"`
	Body        string    `json:"body"`
	BaseRefName string    `json:"baseRefName"`
	HeadRefOID  string    `json:"headRefOid"`
	Title       string    `json:"title"`
	IsDraft     bool      `json:"isDraft"`
	State       string    `json:"state"`
	MergeCommit *GhCommit `json:"mergeCommit"`
}

type GhCommit struct {
	OID string `json:"oid"`
}

func New() *App {
	return &App{in: os.Stdin, stdout: os.Stdout, stderr: os.Stderr}
}

func NewWithIO(in io.Reader, stdout io.Writer, stderr io.Writer) *App {
	if in == nil {
		in = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	return &App{in: in, stdout: stdout, stderr: stderr}
}
