package app

import "testing"

func TestBranchPRState(t *testing.T) {
	tests := []struct {
		name string
		pr   *PRMeta
		want string
	}{
		{name: "local only without pr", pr: nil, want: "local-only"},
		{name: "local only invalid pr", pr: &PRMeta{}, want: "local-only"},
		{name: "submitted", pr: &PRMeta{Number: 42, Updated: false}, want: "submitted"},
		{name: "updated", pr: &PRMeta{Number: 42, Updated: true}, want: "updated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := branchPRState(tt.pr); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
