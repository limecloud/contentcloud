package domain

import "testing"

func TestValidJSONPointerRejectsMalformedEscape(t *testing.T) {
	for _, pointer := range []string{"/title", "/shots/0/voiceover", "/a~1b"} {
		if !ValidJSONPointer(pointer) {
			t.Fatalf("valid pointer rejected: %s", pointer)
		}
	}
	for _, pointer := range []string{"", "title", "/bad~", "/bad~2escape"} {
		if ValidJSONPointer(pointer) {
			t.Fatalf("invalid pointer accepted: %s", pointer)
		}
	}
}
