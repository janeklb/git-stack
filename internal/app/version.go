package app

import "fmt"

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

func formatVersion() string {
	return fmt.Sprintf("version=%s commit=%s date=%s", buildVersion, buildCommit, buildDate)
}
