package app

import (
	"fmt"
	"runtime/debug"
)

var (
	buildVersion  = "dev"
	buildCommit   = "none"
	buildDate     = "unknown"
	readBuildInfo = debug.ReadBuildInfo
)

func formatVersion() string {
	version, commit, date := resolvedBuildMetadata()
	return fmt.Sprintf("version=%s commit=%s date=%s", version, commit, date)
}

func resolvedBuildMetadata() (string, string, string) {
	version := buildVersion
	commit := buildCommit
	date := buildDate

	info, ok := readBuildInfo()
	if !ok {
		return version, commit, date
	}

	settings := parseBuildSettings(info.Settings)

	if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	if commit == "none" && settings.vcsRevision != "" {
		commit = settings.vcsRevision
		if settings.vcsModified {
			commit += ".uncommitted"
		}
	}
	if date == "unknown" && settings.vcsTime != "" {
		date = settings.vcsTime
	}

	return version, commit, date
}

type buildSettings struct {
	vcsRevision string
	vcsModified bool
	vcsTime     string
}

func parseBuildSettings(settings []debug.BuildSetting) buildSettings {
	var values buildSettings
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			values.vcsRevision = setting.Value
		case "vcs.modified":
			values.vcsModified = setting.Value == "true"
		case "vcs.time":
			values.vcsTime = setting.Value
		}
	}
	return values
}
