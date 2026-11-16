package app

type forwardGitClient interface {
	ListOriginBranches() ([]string, error)
	RemoteBranchExists(branch string) (bool, error)
	LocalBranchExists(branch string) bool
	CurrentBranch() (string, error)
	Run(args ...string) error
	DeleteLocalBranch(branch string) error
}

type forwardGHClient interface {
	FindByHead(branch string) (*GhPR, error)
	FindMergedByHead(branch string) (*GhPR, error)
	View(number int) (*GhPR, error)
}

type pruneGitClient interface {
	ListLocalBranches() ([]string, error)
	ListOriginBranches() ([]string, error)
	RemoteBranchExists(branch string) (bool, error)
	EnsurePRHeadCommit(number int, commit string) error
	BranchAtOrBehindCommit(branch, commit string) (bool, error)
	BaseContainsCommit(base, commit string) (bool, error)
	BranchFullyIntegrated(branch, base string) (bool, error)
}

type pruneGHClient interface {
	FindMergedByHead(branch string) (*GhPR, error)
	View(number int) (*GhPR, error)
}

type submitGitClient interface {
	PushBranch(branch string) error
	RemoteBranchExists(branch string) (bool, error)
	LocalBranchExists(branch string) bool
	BranchHasCommitsSince(base, branch string) (bool, error)
	CurrentBranch() (string, error)
	Run(args ...string) error
	DeleteLocalBranch(branch string) error
	BranchFullyIntegrated(branch, base string) (bool, error)
}

type submitGHClient interface {
	FindByHead(branch string) (*GhPR, error)
	FindMergedByHead(branch string) (*GhPR, error)
	View(number int) (*GhPR, error)
}
