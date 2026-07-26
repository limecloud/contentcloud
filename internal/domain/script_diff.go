package domain

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

func ValidateScriptChange(baseline, candidate ScriptPackage, request ScriptChangeRequest) ([]string, ValidationReport) {
	report := ValidationReport{Valid: true, Errors: []ValidationIssue{}, Warnings: []ValidationIssue{}}
	add := func(path, code, message string) {
		report.Valid = false
		report.Errors = append(report.Errors, ValidationIssue{Path: path, Code: code, Message: message})
	}
	changed, err := ChangedJSONPointers(baseline, candidate)
	if err != nil {
		add("/", "SCRIPT_DIFF_FAILED", "无法计算剧本版本差异")
		return nil, report
	}
	if len(changed) == 0 {
		add("/", "SCRIPT_CHANGE_EMPTY", "新版本必须包含至少一项内容变化")
	}
	for _, invariant := range request.InvariantFields {
		if !ValidJSONPointer(invariant) {
			add("/change_request/invariant_fields", "INVARIANT_POINTER_INVALID", "保留字段必须使用 JSON Pointer")
			continue
		}
		if anyPointerWithin(changed, invariant) {
			add(invariant, "SCRIPT_INVARIANT_CHANGED", "声明为保留项的字段发生了变化")
		}
	}
	for _, expected := range request.ChangedFields {
		if !ValidJSONPointer(expected) {
			add("/change_request/changed_fields", "CHANGED_POINTER_INVALID", "变化字段必须使用 JSON Pointer")
			continue
		}
		if !anyPointerWithin(changed, expected) {
			add(expected, "EXPECTED_CHANGE_MISSING", "声明的变化字段没有发生变化")
		}
	}
	if request.ChangeType == "variant" {
		if len(request.ChangedFields) != 1 {
			add("/change_request/changed_fields", "VARIANT_SINGLE_VARIABLE_REQUIRED", "单变量变体必须且只能声明一个变化字段")
		} else {
			for _, pointer := range changed {
				if !pointerWithin(pointer, request.ChangedFields[0]) {
					add(pointer, "VARIANT_UNDECLARED_CHANGE", "单变量变体改变了声明字段之外的内容")
				}
			}
		}
		if strings.TrimSpace(request.Hypothesis) == "" {
			add("/change_request/hypothesis", "VARIANT_HYPOTHESIS_REQUIRED", "单变量变体必须说明实验假设")
		}
	}
	return changed, report
}

func ChangedJSONPointers(before, after any) ([]string, error) {
	left, err := json.Marshal(before)
	if err != nil {
		return nil, err
	}
	right, err := json.Marshal(after)
	if err != nil {
		return nil, err
	}
	var leftValue, rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return nil, err
	}
	changed := []string{}
	diffJSONValue("", leftValue, rightValue, &changed)
	sort.Strings(changed)
	return changed, nil
}

func diffJSONValue(path string, left, right any, changed *[]string) {
	leftObject, leftIsObject := left.(map[string]any)
	rightObject, rightIsObject := right.(map[string]any)
	if leftIsObject && rightIsObject {
		keys := map[string]bool{}
		for key := range leftObject {
			keys[key] = true
		}
		for key := range rightObject {
			keys[key] = true
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			diffJSONValue(path+"/"+escapeJSONPointer(key), leftObject[key], rightObject[key], changed)
		}
		return
	}
	leftArray, leftIsArray := left.([]any)
	rightArray, rightIsArray := right.([]any)
	if leftIsArray && rightIsArray {
		length := len(leftArray)
		if len(rightArray) > length {
			length = len(rightArray)
		}
		for index := 0; index < length; index++ {
			var leftItem, rightItem any
			if index < len(leftArray) {
				leftItem = leftArray[index]
			}
			if index < len(rightArray) {
				rightItem = rightArray[index]
			}
			diffJSONValue(path+"/"+strconv.Itoa(index), leftItem, rightItem, changed)
		}
		return
	}
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	if string(leftJSON) != string(rightJSON) {
		if path == "" {
			path = "/"
		}
		*changed = append(*changed, path)
	}
}

func ValidJSONPointer(pointer string) bool {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return false
	}
	for index := 0; index < len(pointer); index++ {
		if pointer[index] == '~' && (index+1 >= len(pointer) || (pointer[index+1] != '0' && pointer[index+1] != '1')) {
			return false
		}
	}
	return true
}

func anyPointerWithin(changed []string, scope string) bool {
	for _, pointer := range changed {
		if pointerWithin(pointer, scope) {
			return true
		}
	}
	return false
}

func pointerWithin(pointer, scope string) bool {
	return pointer == scope || strings.HasPrefix(pointer, scope+"/")
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func MergeValidationReports(reports ...ValidationReport) ValidationReport {
	result := ValidationReport{Valid: true, Errors: []ValidationIssue{}, Warnings: []ValidationIssue{}}
	for _, report := range reports {
		if !report.Valid {
			result.Valid = false
		}
		result.Errors = append(result.Errors, report.Errors...)
		result.Warnings = append(result.Warnings, report.Warnings...)
	}
	return result
}
