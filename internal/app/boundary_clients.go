package app

type refreshGitBoundary interface {
	RemoteBranchExists(branch string) (bool, error)
	LocalBranchExists(branch string) bool
	CurrentBranch() (string, error)
	Run(args ...string) error
	DeleteLocalBranch(branch string) error
}

type refreshGHBoundary interface {
	View(number int) (*GhPR, error)
}

type pruneGitBoundary interface {
	ListLocalBranches() ([]string, error)
	RemoteBranchExists(branch string) (bool, error)
	BranchAtOrBehindCommit(branch, commit string) (bool, error)
	BaseContainsCommit(base, commit string) (bool, error)
}

type pruneGHBoundary interface {
	FindMergedByHead(branch string) (*GhPR, error)
}

type defaultGitBoundary struct{}

func (defaultGitBoundary) RemoteBranchExists(branch string) (bool, error) {
	return remoteBranchExists(branch)
}

func (defaultGitBoundary) LocalBranchExists(branch string) bool {
	return localBranchExists(branch)
}

func (defaultGitBoundary) CurrentBranch() (string, error) {
	return currentBranch()
}

func (defaultGitBoundary) Run(args ...string) error {
	return gitRun(args...)
}

func (defaultGitBoundary) DeleteLocalBranch(branch string) error {
	return deleteLocalBranch(branch)
}

func (defaultGitBoundary) ListLocalBranches() ([]string, error) {
	return listLocalBranches()
}

func (defaultGitBoundary) BranchAtOrBehindCommit(branch, commit string) (bool, error) {
	return branchAtOrBehindCommit(branch, commit)
}

func (defaultGitBoundary) BaseContainsCommit(base, commit string) (bool, error) {
	return baseContainsCommit(base, commit)
}

type defaultGHBoundary struct{}

func (defaultGHBoundary) View(number int) (*GhPR, error) {
	return ghView(number)
}

func (defaultGHBoundary) FindMergedByHead(branch string) (*GhPR, error) {
	return ghFindMergedByHead(branch)
}
