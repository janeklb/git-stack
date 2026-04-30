package app

import "testing"

func TestBranchStateLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pr   *PRMeta
		want string
	}{
		{name: "local only without pr", pr: nil, want: "local-only"},
		{name: "local only invalid pr", pr: &PRMeta{}, want: "local-only"},
		{name: "pr present without explicit label", pr: &PRMeta{Number: 42, Updated: false}, want: ""},
		{name: "updated pr also has no explicit label", pr: &PRMeta{Number: 42, Updated: true}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := branchStateLabel(tt.pr); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
