package app

import (
	"strings"
	"testing"
)

func TestCmdRefreshRejectsInvalidPublishValue(t *testing.T) {
	err := New().cmdRefresh(false, "invalid")
	if err == nil {
		t.Fatalf("expected refresh to fail for invalid publish scope")
	}
	if !strings.Contains(err.Error(), "--publish must be one of: current, all") {
		t.Fatalf("expected validation message, got: %v", err)
	}
}
