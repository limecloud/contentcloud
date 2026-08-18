package ingest

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"
)

func TestTextEvidenceKeepsLineLocators(t *testing.T) {
	result := Parse("notes.txt", "text/plain", []byte("first\n\nsecond"))
	if result.Status != "ready" || len(result.Evidence) != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestPPTXExtractionKeepsSlideLocator(t *testing.T) {
	var body bytes.Buffer
	zw := zip.NewWriter(&body)
	w, _ := zw.Create("ppt/slides/slide1.xml")
	_, _ = w.Write([]byte(`<p:sld xmlns:p="urn:p" xmlns:a="urn:a"><a:p><a:r><a:t>开场画面</a:t></a:r></a:p></p:sld>`))
	_ = zw.Close()
	result := Parse("deck.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", body.Bytes())
	if result.Status != "ready" || len(result.Evidence) != 1 || result.Evidence[0].Locator["slide"] != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestXLSXExtractionKeepsCellLocator(t *testing.T) {
	var body bytes.Buffer
	zw := zip.NewWriter(&body)
	shared, _ := zw.Create("xl/sharedStrings.xml")
	_, _ = shared.Write([]byte(`<sst><si><t>产品规格</t></si></sst>`))
	sheet, _ := zw.Create("xl/worksheets/sheet1.xml")
	_, _ = sheet.Write([]byte(`<worksheet><sheetData><row><c r="B2" t="s"><v>0</v></c></row></sheetData></worksheet>`))
	_ = zw.Close()
	result := Parse("facts.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", body.Bytes())
	if result.Status != "ready" || len(result.Evidence) != 1 || result.Evidence[0].Locator["cell"] != "B2" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestOCRUnavailableIsReviewable(t *testing.T) {
	original := os.Getenv("PATH")
	t.Setenv("PATH", "")
	result := Parse("image.png", "image/png", []byte("not-an-image"))
	t.Setenv("PATH", original)
	if result.Status != "needs_review" || result.ErrorCode != "OCR_UNAVAILABLE" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDetectMIMERejectsExtensionSpoofing(t *testing.T) {
	if got := DetectMIME([]byte("%PDF-1.7\n")); got != "application/pdf" {
		t.Fatalf("unexpected MIME %s", got)
	}
	if got := DetectMIME([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}); got != "image/png" {
		t.Fatalf("unexpected MIME %s", got)
	}
}

func TestDOCXExtraction(t *testing.T) {
	var body bytes.Buffer
	zw := zip.NewWriter(&body)
	w, _ := zw.Create("word/document.xml")
	_, _ = w.Write([]byte(`<w:document xmlns:w="urn:w"><w:body><w:p><w:r><w:t>金陵</w:t></w:r><w:r><w:t>古香</w:t></w:r></w:p></w:body></w:document>`))
	_ = zw.Close()
	result := Parse("guide.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", body.Bytes())
	if result.Status != "ready" || len(result.Evidence) != 1 || result.Evidence[0].QuoteText != "金陵古香" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
