package app

const (
	stateVersion       = 1
	managedBlockStart  = "<!-- stack:managed:start -->"
	managedBlockEnd    = "<!-- stack:managed:end -->"
	defaultRestackMode = "rebase"
)

type App struct{}

type State struct {
	Version     int                     `json:"version"`
	Trunk       string                  `json:"trunk"`
	RestackMode string                  `json:"restackMode"`
	Naming      NamingConfig            `json:"naming"`
	Branches    map[string]*BranchRef   `json:"branches"`
	Archived    map[string]*ArchivedRef `json:"archived,omitempty"`
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
	Type           string   `json:"type"`
	Mode           string   `json:"mode"`
	OriginalBranch string   `json:"originalBranch"`
	Queue          []string `json:"queue"`
	Index          int      `json:"index"`
}

type GhPR struct {
	Number      int       `json:"number"`
	URL         string    `json:"url"`
	Body        string    `json:"body"`
	BaseRefName string    `json:"baseRefName"`
	Title       string    `json:"title"`
	State       string    `json:"state"`
	MergeCommit *GhCommit `json:"mergeCommit"`
}

type GhCommit struct {
	OID string `json:"oid"`
}

func New() *App {
	return &App{}
}
