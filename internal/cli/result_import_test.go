package cli

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestParseObservationCSV(t *testing.T) {
	data := []byte("approved_snapshot_id,platform,account_alias,published_at,window_hours,sample_status,impressions,completion_rate,currency,spend,gmv\nsnapshot-1,douyin,brand-main,2026-07-20T12:00:00Z,24,seed_candidate,12000,0.42,CNY,100,300\n")
	items, err := parseObservationFile("results.csv", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Metrics["impressions"] != 12000 || items[0].SampleStatus != "seed_candidate" || items[0].Spend != 100 || items[0].GMV != 300 || items[0].RowNumber != 2 {
		t.Fatalf("unexpected observations: %#v", items)
	}
}

func TestParseObservationXLSX(t *testing.T) {
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	sheet, _ := writer.Create("xl/worksheets/sheet1.xml")
	_, _ = sheet.Write([]byte(`<worksheet><sheetData>
<row><c r="A1" t="inlineStr"><is><t>approved_snapshot_id</t></is></c><c r="B1" t="inlineStr"><is><t>platform</t></is></c><c r="C1" t="inlineStr"><is><t>account_alias</t></is></c><c r="D1" t="inlineStr"><is><t>published_at</t></is></c><c r="E1" t="inlineStr"><is><t>window_hours</t></is></c><c r="F1" t="inlineStr"><is><t>sample_status</t></is></c><c r="G1" t="inlineStr"><is><t>views</t></is></c></row>
<row><c r="A2" t="inlineStr"><is><t>snapshot-1</t></is></c><c r="B2" t="inlineStr"><is><t>douyin</t></is></c><c r="C2" t="inlineStr"><is><t>brand-main</t></is></c><c r="D2" t="inlineStr"><is><t>2026-07-20T12:00:00Z</t></is></c><c r="E2"><v>24</v></c><c r="F2" t="inlineStr"><is><t>insufficient_sample</t></is></c><c r="G2"><v>42</v></c></row>
</sheetData></worksheet>`))
	_ = writer.Close()
	items, err := parseObservationFile("results.xlsx", body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Metrics["views"] != 42 {
		t.Fatalf("unexpected observations: %#v", items)
	}
}

func TestParseObservationRejectsSpreadsheetFormula(t *testing.T) {
	data := []byte("approved_snapshot_id,platform,account_alias,published_at,window_hours,sample_status\nsnapshot-1,douyin,=1+1,2026-07-20T12:00:00Z,24,insufficient_sample\n")
	if _, err := parseObservationFile("results.csv", data); err == nil {
		t.Fatal("spreadsheet formula must be rejected")
	}
}

func TestParseObservationRejectsMissingColumns(t *testing.T) {
	_, err := parseObservationFile("results.csv", []byte("platform,views\ndouyin,1\n"))
	if err == nil {
		t.Fatal("missing required columns must fail")
	}
}

func TestResultCommandSchemasCoverAtomicImportAndRating(t *testing.T) {
	schemas := commandSchemas()
	for _, name := range []string{"result.list", "result.import", "result.batches", "result.batch-show", "result.rate", "result.ratings"} {
		if schemas[name] == nil {
			t.Fatalf("missing command schema for %s", name)
		}
	}
}
