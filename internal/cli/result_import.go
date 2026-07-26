package cli

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func parseObservationFile(name string, data []byte) ([]app.CreateObservationInput, error) {
	switch {
	case strings.HasSuffix(strings.ToLower(name), ".json"):
		return decodeObservationJSON(data)
	case strings.HasSuffix(strings.ToLower(name), ".csv"):
		reader := csv.NewReader(strings.NewReader(string(data)))
		reader.FieldsPerRecord = -1
		rows, err := reader.ReadAll()
		if err != nil {
			return nil, domain.Invalid("RESULT_INPUT_INVALID", "CSV 无法解析")
		}
		return observationsFromRows(rows)
	case strings.HasSuffix(strings.ToLower(name), ".xlsx"):
		rows, err := readXLSXRows(data)
		if err != nil {
			return nil, domain.Invalid("RESULT_INPUT_INVALID", "XLSX 无法解析: "+err.Error())
		}
		return observationsFromRows(rows)
	default:
		return nil, domain.Invalid("RESULT_INPUT_INVALID", "结果文件必须是 JSON、CSV 或 XLSX")
	}
}

func decodeObservationJSON(data []byte) ([]app.CreateObservationInput, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, domain.Invalid("RESULT_INPUT_INVALID", "结果 JSON 不能为空")
	}
	decode := func(target any) error {
		decoder := json.NewDecoder(bytes.NewReader(trimmed))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			return err
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return fmt.Errorf("JSON 包含多余内容")
		}
		return nil
	}
	if trimmed[0] == '[' {
		var many []app.CreateObservationInput
		if err := decode(&many); err != nil {
			return nil, domain.Invalid("RESULT_INPUT_INVALID", "结果 JSON 数组无效或包含未知字段: "+err.Error())
		}
		return many, nil
	}
	var one app.CreateObservationInput
	if err := decode(&one); err != nil {
		return nil, domain.Invalid("RESULT_INPUT_INVALID", "结果 JSON 对象无效或包含未知字段: "+err.Error())
	}
	return []app.CreateObservationInput{one}, nil
}

func observationsFromRows(rows [][]string) ([]app.CreateObservationInput, error) {
	if len(rows) < 2 {
		return nil, domain.Invalid("RESULT_INPUT_INVALID", "结果表至少需要表头和一行数据")
	}
	headers := make([]string, len(rows[0]))
	for i, header := range rows[0] {
		headers[i] = strings.ToLower(strings.TrimSpace(header))
	}
	required := map[string]bool{"platform": false, "account_alias": false, "published_at": false, "window_hours": false, "sample_status": false}
	hasVersionColumn := false
	for _, header := range headers {
		if header == "approved_snapshot_id" || header == "script_version_id" {
			hasVersionColumn = true
		}
		if _, ok := required[header]; ok {
			required[header] = true
		}
	}
	if !hasVersionColumn {
		return nil, domain.Invalid("RESULT_COLUMN_REQUIRED", "结果表缺少列: approved_snapshot_id")
	}
	for name, present := range required {
		if !present {
			return nil, domain.Invalid("RESULT_COLUMN_REQUIRED", "结果表缺少列: "+name)
		}
	}
	metricNames := map[string]bool{"impressions": true, "views": true, "likes": true, "comments": true, "shares": true, "three_second_retention_rate": true, "completion_rate": true, "clicks": true, "conversions": true}
	out := make([]app.CreateObservationInput, 0, len(rows)-1)
	rowErrors := []domain.PerformanceImportRowError{}
	for index, row := range rows[1:] {
		if len(row) == 0 || strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		rowNumber := index + 2
		input := app.CreateObservationInput{RowNumber: rowNumber, Metrics: map[string]float64{}}
		for column, header := range headers {
			value := ""
			if column < len(row) {
				value = strings.TrimSpace(row[column])
			}
			if dangerousSheetCell(value) {
				rowErrors = append(rowErrors, domain.PerformanceImportRowError{RowNumber: rowNumber, Field: header, Code: "RESULT_FORMULA_INJECTION", Message: "单元格不能包含电子表格公式"})
				continue
			}
			switch header {
			case "project_id":
				input.ProjectID = value
			case "script_version_id":
				input.ScriptVersionID = value
			case "approved_snapshot_id":
				input.ApprovedSnapshotID = value
			case "platform":
				input.Platform = value
			case "account_alias":
				input.AccountAlias = value
			case "published_at":
				parsed, err := time.Parse(time.RFC3339, value)
				if err != nil {
					rowErrors = append(rowErrors, domain.PerformanceImportRowError{RowNumber: rowNumber, Field: header, Code: "RESULT_DATE_INVALID", Message: "published_at 必须为 RFC3339"})
				} else {
					input.PublishedAt = parsed
				}
			case "window_hours":
				parsed, err := strconv.Atoi(value)
				if err != nil {
					rowErrors = append(rowErrors, domain.PerformanceImportRowError{RowNumber: rowNumber, Field: header, Code: "RESULT_WINDOW_INVALID", Message: "window_hours 必须是整数"})
				} else {
					input.WindowHours = parsed
				}
			case "sample_status":
				input.SampleStatus = value
			case "currency":
				input.Currency = value
			case "spend":
				parsed, err := parseOptionalNumber(value)
				if err != nil {
					rowErrors = append(rowErrors, domain.PerformanceImportRowError{RowNumber: rowNumber, Field: header, Code: "RESULT_SPEND_INVALID", Message: "spend 必须是数字"})
				} else {
					input.Spend = parsed
				}
			case "gmv":
				parsed, err := parseOptionalNumber(value)
				if err != nil {
					rowErrors = append(rowErrors, domain.PerformanceImportRowError{RowNumber: rowNumber, Field: header, Code: "RESULT_GMV_INVALID", Message: "gmv 必须是数字"})
				} else {
					input.GMV = parsed
				}
			case "roi":
				if value != "" {
					parsed, err := strconv.ParseFloat(value, 64)
					if err != nil {
						rowErrors = append(rowErrors, domain.PerformanceImportRowError{RowNumber: rowNumber, Field: header, Code: "RESULT_ROI_INVALID", Message: "roi 必须是数字"})
					} else {
						input.SubmittedROI = &parsed
					}
				}
			case "issue_category":
				input.IssueCategory = value
			case "notes":
				input.Notes = value
			default:
				metricName := strings.TrimPrefix(header, "metric.")
				if (metricNames[header] || strings.HasPrefix(header, "metric.")) && value != "" {
					number, err := strconv.ParseFloat(value, 64)
					if err != nil {
						rowErrors = append(rowErrors, domain.PerformanceImportRowError{RowNumber: rowNumber, Field: header, Code: "RESULT_METRIC_INVALID", Message: fmt.Sprintf("指标 %s 不是数字", metricName)})
						continue
					}
					input.Metrics[metricName] = number
				}
			}
		}
		out = append(out, input)
	}
	if len(rowErrors) > 0 {
		err := domain.Invalid("RESULT_INPUT_INVALID", "结果文件包含无效单元格")
		err.Details = map[string]any{"row_errors": rowErrors}
		return nil, err
	}
	return out, nil
}

func parseOptionalNumber(value string) (float64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value, 64)
}

func dangerousSheetCell(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && strings.ContainsRune("=+@-", rune(value[0]))
}

type xlsxSharedStrings struct {
	Items []struct {
		Text string `xml:"t"`
		Runs []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

type xlsxSheet struct {
	Rows []struct {
		Cells []struct {
			Ref     string `xml:"r,attr"`
			Type    string `xml:"t,attr"`
			Formula string `xml:"f"`
			Value   string `xml:"v"`
			Text    string `xml:"is>t"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func readXLSXRows(data []byte) ([][]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var shared []string
	var sheetData []byte
	for _, file := range reader.File {
		if file.UncompressedSize64 > 20*1024*1024 {
			return nil, fmt.Errorf("工作表解压后过大")
		}
		if file.Name != "xl/sharedStrings.xml" && file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		body, err := readZipEntry(file)
		if err != nil {
			return nil, err
		}
		if file.Name == "xl/sharedStrings.xml" {
			var parsed xlsxSharedStrings
			if err := xml.Unmarshal(body, &parsed); err != nil {
				return nil, err
			}
			for _, item := range parsed.Items {
				value := item.Text
				for _, run := range item.Runs {
					value += run.Text
				}
				shared = append(shared, value)
			}
		} else {
			sheetData = body
		}
	}
	if len(sheetData) == 0 {
		return nil, fmt.Errorf("缺少第一个工作表")
	}
	var sheet xlsxSheet
	if err := xml.Unmarshal(sheetData, &sheet); err != nil {
		return nil, err
	}
	rows := make([][]string, 0, len(sheet.Rows))
	for _, sourceRow := range sheet.Rows {
		row := []string{}
		for _, cell := range sourceRow.Cells {
			column := xlsxColumnIndex(cell.Ref)
			for len(row) <= column {
				row = append(row, "")
			}
			value := cell.Value
			if cell.Formula != "" {
				value = "=" + cell.Formula
			}
			if cell.Type == "inlineStr" {
				value = cell.Text
			} else if cell.Type == "s" {
				index, _ := strconv.Atoi(value)
				if index >= 0 && index < len(shared) {
					value = shared[index]
				}
			}
			row[column] = value
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func readZipEntry(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, 20*1024*1024+1))
}

func xlsxColumnIndex(ref string) int {
	index := 0
	for _, char := range ref {
		if char < 'A' || char > 'Z' {
			break
		}
		index = index*26 + int(char-'A'+1)
	}
	if index == 0 {
		return 0
	}
	return index - 1
}
