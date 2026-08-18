package application

import "testing"

func TestChecksPassedRequiresEveryDeclaredCheck(t *testing.T) {
	tests := []struct {
		name   string
		checks map[string]any
		names  []string
		want   bool
	}{
		{name: "all checks pass", checks: map[string]any{"content.schema": true, "claim.references": "passed"}, names: []string{"content.schema", "claim.references"}, want: true},
		{name: "missing check blocks", checks: map[string]any{"content.schema": true}, names: []string{"content.schema", "claim.references"}, want: false},
		{name: "failed check blocks", checks: map[string]any{"content.schema": true, "claim.references": false}, names: []string{"content.schema", "claim.references"}, want: false},
		{name: "aggregate compatibility without declared checks", checks: map[string]any{"passed": true}, names: nil, want: true},
		{name: "aggregate flag cannot replace declared checks", checks: map[string]any{"passed": true}, names: []string{"content.schema"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := checksPassed(test.checks, test.names); got != test.want {
				t.Fatalf("checksPassed(%#v, %#v) = %v, want %v", test.checks, test.names, got, test.want)
			}
		})
	}
}
