package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestJianyingExportCommandContractMatchesSchema(t *testing.T) {
	schema, ok := commandSchemas()["jianying.export"].(map[string]any)
	if !ok {
		t.Fatalf("jianying.export schema missing: %#v", commandSchemas()["jianying.export"])
	}
	arguments, _ := schema["arguments"].([]string)
	for _, required := range []string{"project", "approved-snapshot-id", "final-review-id", "delivery-package-id", "manifest"} {
		found := false
		for _, argument := range arguments {
			if argument == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("schema arguments=%#v missing %q", arguments, required)
		}
	}

	root := (&Root{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}).command()
	command, _, err := root.Find([]string{"artifact", "jianying-export"})
	if err != nil || command == nil {
		t.Fatalf("jianying-export command is not executable: command=%#v error=%v", command, err)
	}
	for _, flag := range []string{"project", "approved-snapshot-id", "final-review-id", "delivery-package-id", "manifest", "out"} {
		if command.Flags().Lookup(flag) == nil {
			t.Fatalf("jianying-export command missing --%s", flag)
		}
	}
	for _, required := range []string{"project", "approved-snapshot-id", "final-review-id", "delivery-package-id", "manifest"} {
		values := command.Flag(required).Annotations[cobra.BashCompOneRequiredFlag]
		if len(values) != 1 || values[0] != "true" {
			t.Fatalf("--%s is not marked required", required)
		}
	}
}
