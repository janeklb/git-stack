package app

import (
	"strings"
	"testing"
)

func TestUpsertManagedBlockAppendsAndReplaces(t *testing.T) {
	managedOne := managedBlock("feat-one", "main")
	body := upsertManagedBlock("User body", managedOne)
	if !strings.Contains(body, "User body") {
		t.Fatalf("expected original body to be preserved, got:\n%s", body)
	}
	if !strings.Contains(body, "branch: feat-one") {
		t.Fatalf("expected managed block to be appended, got:\n%s", body)
	}

	managedTwo := managedBlock("feat-one", "feat-zero")
	replaced := upsertManagedBlock(body, managedTwo)
	if strings.Contains(replaced, "parent: main") {
		t.Fatalf("expected old managed block to be replaced, got:\n%s", replaced)
	}
	if !strings.Contains(replaced, "parent: feat-zero") {
		t.Fatalf("expected new managed block, got:\n%s", replaced)
	}
}
