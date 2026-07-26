package exportfmt

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestXLSXEscapesFormulaPrefixes(t *testing.T) {
	body, err := XLSX("镜头", [][]string{{"=SUM(A1:A2)", "+1", "-1", "@value", "ordinary"}})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	var sheet string
	for _, file := range reader.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		sheet = string(contents)
	}
	for _, value := range []string{"&#39;=SUM(A1:A2)", "&#39;+1", "&#39;-1", "&#39;@value", "ordinary"} {
		if !strings.Contains(sheet, value) {
			t.Fatalf("worksheet did not preserve escaped value %q: %s", value, sheet)
		}
	}
}
