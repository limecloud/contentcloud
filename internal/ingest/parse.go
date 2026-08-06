package ingest

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	pdf "rsc.io/pdf"
)

const ParserVersion = "contentcloud-ingest/1.0.0"

type Result struct {
	Status    string
	ErrorCode string
	Evidence  []Evidence
}

type Evidence struct {
	LocatorKind   string
	Locator       map[string]any
	QuoteText     string
	OCRConfidence *float64
}

func DetectMIME(data []byte) string {
	switch {
	case len(data) >= 5 && string(data[:5]) == "%PDF-":
		return "application/pdf"
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return "image/png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg"
	case len(data) >= 4 && bytes.Equal(data[:4], []byte{'P', 'K', 0x03, 0x04}):
		files, err := zipFiles(data)
		if err != nil {
			return "application/zip"
		}
		if _, ok := files["word/document.xml"]; ok {
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		}
		if _, ok := files["ppt/presentation.xml"]; ok {
			return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
		}
		for name := range files {
			if strings.HasPrefix(name, "ppt/slides/slide") {
				return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
			}
			if strings.HasPrefix(name, "xl/worksheets/sheet") {
				return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
			}
		}
		return "application/zip"
	case utf8.Valid(data) && !bytes.ContainsRune(data, '\x00'):
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func Parse(fileName, mimeType string, data []byte) Result {
	var evidence []Evidence
	var err error
	switch mimeType {
	case "text/plain":
		evidence = textEvidence(string(data))
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		evidence, err = parseDOCX(data)
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		evidence, err = parsePPTX(data)
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		evidence, err = parseXLSX(data)
	case "application/pdf":
		evidence, err = parsePDF(data)
	case "image/png", "image/jpeg":
		evidence, err = parseImageOCR(fileName, data)
	default:
		err = fmt.Errorf("不支持的 MIME 类型 %s", mimeType)
	}
	if err != nil {
		return Result{Status: "needs_review", ErrorCode: classifyError(err), Evidence: evidence}
	}
	if len(evidence) == 0 {
		return Result{Status: "needs_review", ErrorCode: "NO_TEXT_EXTRACTED", Evidence: []Evidence{}}
	}
	status := "ready"
	for _, span := range evidence {
		if span.OCRConfidence != nil && *span.OCRConfidence < 0.85 {
			status = "needs_review"
			break
		}
	}
	return Result{Status: status, Evidence: evidence}
}

func textEvidence(value string) []Evidence {
	out := []Evidence{}
	scanner := bufio.NewScanner(strings.NewReader(value))
	line := 0
	for scanner.Scan() {
		line++
		quote := strings.TrimSpace(scanner.Text())
		if quote == "" {
			continue
		}
		out = append(out, Evidence{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": line}, QuoteText: quote})
	}
	return out
}

func zipFiles(data []byte) (map[string][]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	var total uint64
	for _, file := range reader.File {
		total += file.UncompressedSize64
		if total > 250*1024*1024 || file.UncompressedSize64 > 100*1024*1024 {
			return nil, fmt.Errorf("压缩包解压后超过大小限制")
		}
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(stream, 100*1024*1024+1))
		_ = stream.Close()
		if err != nil {
			return nil, err
		}
		out[file.Name] = body
	}
	return out, nil
}

func parseDOCX(data []byte) ([]Evidence, error) {
	files, err := zipFiles(data)
	if err != nil {
		return nil, err
	}
	body, ok := files["word/document.xml"]
	if !ok {
		return nil, fmt.Errorf("DOCX 文件缺少 word/document.xml")
	}
	paragraphs, err := xmlParagraphs(body, "p", "t")
	if err != nil {
		return nil, err
	}
	out := []Evidence{}
	for i, quote := range paragraphs {
		if quote != "" {
			out = append(out, Evidence{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": i + 1}, QuoteText: quote})
		}
	}
	return out, nil
}

func parsePPTX(data []byte) ([]Evidence, error) {
	files, err := zipFiles(data)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for name := range files {
		if strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml") {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return numericSuffix(names[i]) < numericSuffix(names[j]) })
	out := []Evidence{}
	for index, name := range names {
		texts, err := xmlParagraphs(files[name], "p", "t")
		if err != nil {
			return nil, err
		}
		quote := strings.TrimSpace(strings.Join(texts, " "))
		if quote != "" {
			out = append(out, Evidence{LocatorKind: "slide", Locator: map[string]any{"slide": index + 1}, QuoteText: quote})
		}
	}
	return out, nil
}

func parseXLSX(data []byte) ([]Evidence, error) {
	files, err := zipFiles(data)
	if err != nil {
		return nil, err
	}
	shared := []string{}
	if body, ok := files["xl/sharedStrings.xml"]; ok {
		shared, err = xmlParagraphs(body, "si", "t")
		if err != nil {
			return nil, err
		}
	}
	names := []string{}
	for name := range files {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return numericSuffix(names[i]) < numericSuffix(names[j]) })
	out := []Evidence{}
	for sheetIndex, name := range names {
		cells, err := worksheetCells(files[name], shared)
		if err != nil {
			return nil, err
		}
		for _, cell := range cells {
			if cell.Value == "" {
				continue
			}
			out = append(out, Evidence{LocatorKind: "sheet_cell", Locator: map[string]any{"sheet": sheetIndex + 1, "cell": cell.Ref}, QuoteText: cell.Value})
		}
	}
	return out, nil
}

type worksheetCell struct{ Ref, Value string }

func worksheetCells(body []byte, shared []string) ([]worksheetCell, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	out := []worksheetCell{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "c" {
			continue
		}
		var cell struct {
			Type   string `xml:"t,attr"`
			Value  string `xml:"v"`
			Inline struct {
				Text string `xml:"t"`
			} `xml:"is"`
		}
		ref, kind := "", ""
		for _, attr := range start.Attr {
			if attr.Name.Local == "r" {
				ref = attr.Value
			}
			if attr.Name.Local == "t" {
				kind = attr.Value
			}
		}
		if err := decoder.DecodeElement(&cell, &start); err != nil {
			return nil, err
		}
		value := cell.Value
		if kind == "s" {
			index, _ := strconv.Atoi(cell.Value)
			if index >= 0 && index < len(shared) {
				value = shared[index]
			}
		} else if kind == "inlineStr" {
			value = cell.Inline.Text
		}
		out = append(out, worksheetCell{Ref: ref, Value: strings.TrimSpace(value)})
	}
	return out, nil
}

func parsePDF(data []byte) ([]Evidence, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	out := []Evidence{}
	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		page := reader.Page(pageNumber)
		if page.V.IsNull() {
			continue
		}
		texts := page.Content().Text
		sort.Slice(texts, func(i, j int) bool {
			if texts[i].Y == texts[j].Y {
				return texts[i].X < texts[j].X
			}
			return texts[i].Y > texts[j].Y
		})
		parts := []string{}
		for _, text := range texts {
			value := strings.TrimSpace(text.S)
			if value != "" {
				parts = append(parts, value)
			}
		}
		quote := strings.Join(parts, " ")
		if quote != "" {
			out = append(out, Evidence{LocatorKind: "page", Locator: map[string]any{"page": pageNumber}, QuoteText: quote})
		}
	}
	return out, nil
}

func parseImageOCR(fileName string, data []byte) ([]Evidence, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return nil, fmt.Errorf("Tesseract 不可用")
	}
	dir, err := os.MkdirTemp("", "contentcloud-ocr-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "input"+strings.ToLower(filepath.Ext(fileName)))
	if err := os.WriteFile(input, data, 0o600); err != nil {
		return nil, err
	}
	cmd := exec.Command("tesseract", input, "stdout", "-l", "chi_sim+eng", "tsv")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=" + os.Getenv("LANG")}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Tesseract 执行失败：%w", err)
	}
	reader := csv.NewReader(bytes.NewReader(output))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	type line struct {
		words                    []string
		confidence               float64
		count                    int
		left, top, right, bottom int
	}
	lines := map[string]*line{}
	order := []string{}
	for index, row := range records {
		if index == 0 || len(row) < 12 || strings.TrimSpace(row[11]) == "" {
			continue
		}
		key := strings.Join(row[1:5], ":")
		value, ok := lines[key]
		if !ok {
			value = &line{}
			lines[key] = value
			order = append(order, key)
		}
		confidence, _ := strconv.ParseFloat(row[10], 64)
		left, _ := strconv.Atoi(row[6])
		top, _ := strconv.Atoi(row[7])
		width, _ := strconv.Atoi(row[8])
		height, _ := strconv.Atoi(row[9])
		value.words = append(value.words, row[11])
		value.confidence += confidence / 100
		value.count++
		if value.count == 1 || left < value.left {
			value.left = left
		}
		if value.count == 1 || top < value.top {
			value.top = top
		}
		if left+width > value.right {
			value.right = left + width
		}
		if top+height > value.bottom {
			value.bottom = top + height
		}
	}
	out := []Evidence{}
	for _, key := range order {
		value := lines[key]
		confidence := value.confidence / float64(value.count)
		out = append(out, Evidence{LocatorKind: "image_region", Locator: map[string]any{"x": value.left, "y": value.top, "width": value.right - value.left, "height": value.bottom - value.top}, QuoteText: strings.Join(value.words, " "), OCRConfidence: &confidence})
	}
	return out, nil
}

func xmlParagraphs(body []byte, containerName, textName string) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	out := []string{}
	depth := 0
	var current strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == containerName {
				depth++
				if depth == 1 {
					current.Reset()
				}
			}
			if depth > 0 && value.Name.Local == textName {
				var text string
				if err := decoder.DecodeElement(&text, &value); err != nil {
					return nil, err
				}
				current.WriteString(text)
			}
		case xml.EndElement:
			if value.Name.Local == containerName && depth > 0 {
				if depth == 1 {
					out = append(out, strings.TrimSpace(current.String()))
				}
				depth--
			}
		}
	}
	return out, nil
}

func numericSuffix(value string) int {
	base := strings.TrimSuffix(filepath.Base(value), filepath.Ext(value))
	index := len(base)
	for index > 0 && base[index-1] >= '0' && base[index-1] <= '9' {
		index--
	}
	number, _ := strconv.Atoi(base[index:])
	return number
}

func classifyError(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "tesseract"):
		return "OCR_UNAVAILABLE"
	case strings.Contains(message, "expansion limit"):
		return "ARCHIVE_LIMIT_EXCEEDED"
	case strings.Contains(message, "unsupported"):
		return "PARSER_UNSUPPORTED"
	default:
		return "PARSER_FAILED"
	}
}
