package app

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestRenderXLSXFreezesHeaderAndEscapesFormula(t *testing.T) {
	script := domain.ScriptVersion{Version: 1, Package: domain.ScriptPackage{Shots: []domain.Shot{{ShotID: "shot-1", Voiceover: "=HYPERLINK(\"https://example.com\")", AcceptanceCriteria: []string{"visible"}}}}}
	data, err := renderXLSX(script)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var sheet string
	for _, file := range reader.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		stream, _ := file.Open()
		body, _ := io.ReadAll(stream)
		_ = stream.Close()
		sheet = string(body)
	}
	if !strings.Contains(sheet, `state="frozen"`) {
		t.Fatal("first row must be frozen")
	}
	if strings.Contains(sheet, `<t>=HYPERLINK`) || !strings.Contains(sheet, `<t>&#39;=HYPERLINK`) {
		t.Fatal("formula-like cells must be escaped")
	}
}
