package app

import (
	"runtime/debug"
	"testing"
)

func TestResolvedBuildMetadataFallsBackToBuildInfo(t *testing.T) {
	origVersion := buildVersion
	origCommit := buildCommit
	origDate := buildDate
	origReadBuildInfo := readBuildInfo
	t.Cleanup(func() {
		buildVersion = origVersion
		buildCommit = origCommit
		buildDate = origDate
		readBuildInfo = origReadBuildInfo
	})

	buildVersion = "dev"
	buildCommit = "none"
	buildDate = "unknown"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "v0.2.2"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "2444db9d19e39a1ea2a2d2bb9cca016a786de9aa"},
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.time", Value: "2026-05-08T23:33:56Z"},
			},
		}, true
	}

	version, commit, date := resolvedBuildMetadata()
	if version != "v0.2.2" {
		t.Fatalf("expected fallback version, got %q", version)
	}
	if commit != "2444db9d19e39a1ea2a2d2bb9cca016a786de9aa.uncommitted" {
		t.Fatalf("expected fallback commit with dirty suffix, got %q", commit)
	}
	if date != "2026-05-08T23:33:56Z" {
		t.Fatalf("expected fallback date, got %q", date)
	}
}

func TestResolvedBuildMetadataPrefersInjectedValues(t *testing.T) {
	origVersion := buildVersion
	origCommit := buildCommit
	origDate := buildDate
	origReadBuildInfo := readBuildInfo
	t.Cleanup(func() {
		buildVersion = origVersion
		buildCommit = origCommit
		buildDate = origDate
		readBuildInfo = origReadBuildInfo
	})

	buildVersion = "v9.9.9"
	buildCommit = "release-commit"
	buildDate = "2026-05-09T12:00:00Z"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "v0.2.2"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "ignored"},
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.time", Value: "ignored"},
			},
		}, true
	}

	version, commit, date := resolvedBuildMetadata()
	if version != "v9.9.9" {
		t.Fatalf("expected injected version, got %q", version)
	}
	if commit != "release-commit" {
		t.Fatalf("expected injected commit, got %q", commit)
	}
	if date != "2026-05-09T12:00:00Z" {
		t.Fatalf("expected injected date, got %q", date)
	}
}
