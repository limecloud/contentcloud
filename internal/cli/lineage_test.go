package cli

import "testing"

func TestLineageAndAuditCommandSchemas(t *testing.T) {
	schemas := commandSchemas()
	for _, command := range []string{"lineage.show", "lineage.impact", "audit.list"} {
		if schemas[command] == nil {
			t.Fatalf("missing schema for %s", command)
		}
	}
	root := (&Root{}).command()
	for _, args := range [][]string{{"lineage", "show", "--help"}, {"lineage", "impact", "--help"}, {"audit", "list", "--help"}} {
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("command help failed for %v: %v", args, err)
		}
	}
}
