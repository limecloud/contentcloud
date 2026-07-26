package httpapi

import (
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
)

func TestArtifactDispatchRejectsLocalExecutionFields(t *testing.T) {
	for _, body := range []string{
		`{"file_name":"preview.html","envelope":{},"capabilities":[],"path":"/private/customer/file"}`,
		`{"open_request_id":"request-1","state":"opened","reason":"","command":"open"}`,
		`{"open_request_id":"request-1","state":"opened","reason":"","args":["--unsafe"]}`,
		`{"open_request_id":"request-1","state":"opened","reason":"","url":"file:///private/customer/file"}`,
		`{"open_request_id":"request-1","state":"opened","reason":"","plugin_id":"private.renderer"}`,
	} {
		var input app.RegisterArtifactInput
		if err := strictDecodeParams([]byte(body), &input); err == nil {
			t.Fatalf("private execution field was accepted: %s", body)
		}
	}
}
