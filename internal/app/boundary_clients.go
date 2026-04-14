package app

type forwardGitClient interface {
	RemoteBranchExists(branch string) (bool, error)
	LocalBranchExists(branch string) bool
	CurrentBranch() (string, error)
	Run(args ...string) error
	DeleteLocalBranch(branch string) error
}

type forwardGHClient interface {
	View(number int) (*GhPR, error)
}

type pruneGitClient interface {
	ListLocalBranches() ([]string, error)
	RemoteBranchExists(branch string) (bool, error)
	BranchAtOrBehindCommit(branch, commit string) (bool, error)
	BaseContainsCommit(base, commit string) (bool, error)
	BranchFullyIntegrated(branch, base string) (bool, error)
}

type pruneGHClient interface {
	FindMergedByHead(branch string) (*GhPR, error)
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
	View(number int) (*GhPR, error)
}

type defaultGitClient struct{}

func (defaultGitClient) RemoteBranchExists(branch string) (bool, error) {
	return remoteBranchExists(branch)
}

func (defaultGitClient) LocalBranchExists(branch string) bool {
	return localBranchExists(branch)
}

func (defaultGitClient) CurrentBranch() (string, error) {
	return currentBranch()
}

func (defaultGitClient) Run(args ...string) error {
	return gitRun(args...)
}

func (defaultGitClient) DeleteLocalBranch(branch string) error {
	return deleteLocalBranch(branch)
}

func (defaultGitClient) ListLocalBranches() ([]string, error) {
	return listLocalBranches()
}

func (defaultGitClient) BranchAtOrBehindCommit(branch, commit string) (bool, error) {
	return branchAtOrBehindCommit(branch, commit)
}

func (defaultGitClient) BaseContainsCommit(base, commit string) (bool, error) {
	return baseContainsCommit(base, commit)
}

func (defaultGitClient) PushBranch(branch string) error {
	return pushBranch(branch)
}

func (defaultGitClient) BranchFullyIntegrated(branch, base string) (bool, error) {
	return branchFullyIntegrated(branch, base)
}

func (defaultGitClient) BranchHasCommitsSince(base, branch string) (bool, error) {
	return branchHasCommitsSince(base, branch)
}

type defaultGHClient struct{}

func (defaultGHClient) View(number int) (*GhPR, error) {
	return ghView(number)
}

func (defaultGHClient) FindMergedByHead(branch string) (*GhPR, error) {
	return ghFindMergedByHead(branch)
}
