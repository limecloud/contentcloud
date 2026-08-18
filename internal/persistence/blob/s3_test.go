package blob

import "testing"

func TestS3KeyScoping(t *testing.T) {
	store := &S3Store{prefix: "contentcloud/v1"}
	key, err := store.key("tenants/t1/projects/p1/file")
	if err != nil || key != "contentcloud/v1/tenants/t1/projects/p1/file" {
		t.Fatalf("unexpected key %q: %v", key, err)
	}
	for _, invalid := range []string{"", "../secret", "a/../../secret", "a\\..\\secret"} {
		if _, err := store.key(invalid); err == nil {
			t.Fatalf("key %q must be rejected", invalid)
		}
	}
}
