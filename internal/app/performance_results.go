package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const maxPerformanceImportRows = 1000

var performanceSampleStatuses = map[string]bool{
	"insufficient_sample": true,
	"seed_candidate":      true,
	"repairable":          true,
	"discarded":           true,
}

var performanceIssueCategories = map[string]bool{
	"":         true,
	"creative": true,
	"account":  true,
	"delivery": true,
	"landing":  true,
}

var performanceMetrics = map[string]bool{
	"impressions":                 true,
	"views":                       true,
	"three_second_retention_rate": true,
	"completion_rate":             true,
	"clicks":                      true,
	"conversions":                 true,
	"likes":                       true,
	"comments":                    true,
	"shares":                      true,
}

var performanceRateMetrics = map[string]bool{
	"three_second_retention_rate": true,
	"completion_rate":             true,
}

type CreateObservationInput struct {
	RowNumber       int                `json:"row_number,omitempty"`
	ProjectID       string             `json:"project_id,omitempty"`
	ScriptVersionID string             `json:"script_version_id"`
	Platform        string             `json:"platform"`
	AccountAlias    string             `json:"account_alias"`
	PublishedAt     time.Time          `json:"published_at"`
	WindowHours     int                `json:"window_hours"`
	SampleStatus    string             `json:"sample_status"`
	Metrics         map[string]float64 `json:"metrics"`
	Currency        string             `json:"currency,omitempty"`
	Spend           float64            `json:"spend,omitempty"`
	GMV             float64            `json:"gmv,omitempty"`
	SubmittedROI    *float64           `json:"roi,omitempty"`
	IssueCategory   string             `json:"issue_category"`
	Notes           string             `json:"notes"`
}

type ImportPerformanceInput struct {
	ProjectID    string                   `json:"project_id"`
	SourceName   string                   `json:"source_name"`
	SourceFormat string                   `json:"source_format"`
	SourceSHA256 string                   `json:"source_sha256,omitempty"`
	Observations []CreateObservationInput `json:"observations"`
	DryRun       bool                     `json:"dry_run,omitempty"`
}

type ImportPerformanceResult struct {
	DryRun       bool                            `json:"dry_run"`
	Batch        domain.PerformanceImportBatch   `json:"batch"`
	Observations []domain.PerformanceObservation `json:"observations"`
}

type CreateRatingDecisionInput struct {
	ProjectID      string   `json:"project_id"`
	SubjectType    string   `json:"subject_type"`
	SubjectID      string   `json:"subject_id"`
	ObservationIDs []string `json:"observation_ids"`
	Rating         string   `json:"rating"`
	Reason         string   `json:"reason"`
	NextAction     string   `json:"next_action"`
	DryRun         bool     `json:"dry_run,omitempty"`
}

type CreateRatingDecisionResult struct {
	DryRun   bool                  `json:"dry_run"`
	Decision domain.RatingDecision `json:"decision"`
}

func (s *Service) ImportPerformanceObservations(ctx context.Context, actor Actor, in ImportPerformanceInput, requestID string) (ImportPerformanceResult, error) {
	result := ImportPerformanceResult{DryRun: in.DryRun, Observations: []domain.PerformanceObservation{}}
	if actor.Type != "user" {
		return result, domain.Policy("USER_ACTOR_REQUIRED", "只有登录用户可以导入业务结果", "使用用户 CLI 凭据或 Web 会话")
	}
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return result, err
	}
	if _, err := s.projectForWrite(ctx, actor, in.ProjectID); err != nil {
		return result, err
	}
	if len(in.Observations) == 0 || len(in.Observations) > maxPerformanceImportRows {
		return result, domain.Invalid("RESULT_IMPORT_SIZE_INVALID", fmt.Sprintf("每批结果必须包含 1 到 %d 行", maxPerformanceImportRows))
	}

	format := strings.ToLower(strings.TrimSpace(in.SourceFormat))
	if format == "" {
		format = "manual"
	}
	if format != "manual" && format != "json" && format != "csv" && format != "xlsx" {
		return result, domain.Invalid("RESULT_SOURCE_FORMAT_INVALID", "结果来源格式必须是 manual、json、csv 或 xlsx")
	}
	sourceName := strings.TrimSpace(in.SourceName)
	if sourceName == "" {
		sourceName = "manual-entry"
	}
	if len(sourceName) > 255 || dangerousSpreadsheetText(sourceName) {
		return result, domain.Invalid("RESULT_SOURCE_NAME_INVALID", "结果来源名称无效")
	}

	now := s.now().UTC()
	errorsByRow := []domain.PerformanceImportRowError{}
	observations := make([]domain.PerformanceObservation, 0, len(in.Observations))
	dedupRows := map[string]int{}
	batchCurrency := ""
	scriptCache := map[string]domain.ScriptVersion{}

	for index, raw := range in.Observations {
		rowNumber := raw.RowNumber
		if rowNumber <= 0 {
			rowNumber = index + 1
		}
		rowErrors := validatePerformanceInput(rowNumber, in.ProjectID, raw)
		script, cached := scriptCache[raw.ScriptVersionID]
		if !cached && strings.TrimSpace(raw.ScriptVersionID) != "" {
			var err error
			script, err = s.store.Script(ctx, actor.TenantID, strings.TrimSpace(raw.ScriptVersionID))
			if err == nil {
				scriptCache[raw.ScriptVersionID] = script
			}
			if err != nil || script.ProjectID != in.ProjectID {
				rowErrors = append(rowErrors, importRowError(rowNumber, "script_version_id", "RESULT_SCRIPT_NOT_FOUND", "剧本版本不存在或不属于当前项目"))
			}
		} else if cached && script.ProjectID != in.ProjectID {
			rowErrors = append(rowErrors, importRowError(rowNumber, "script_version_id", "RESULT_SCRIPT_NOT_FOUND", "剧本版本不存在或不属于当前项目"))
		}

		currency := strings.ToUpper(strings.TrimSpace(raw.Currency))
		if currency != "" {
			if batchCurrency == "" {
				batchCurrency = currency
			} else if currency != batchCurrency {
				rowErrors = append(rowErrors, importRowError(rowNumber, "currency", "RESULT_MIXED_CURRENCY", "同一导入批次不能混用币种"))
			}
		}

		publishedAt := raw.PublishedAt.UTC()
		dedupKey := performanceDedupKey(in.ProjectID, raw.ScriptVersionID, raw.Platform, raw.AccountAlias, publishedAt, raw.WindowHours)
		if previousRow, duplicate := dedupRows[dedupKey]; duplicate {
			rowErrors = append(rowErrors, importRowError(rowNumber, "", "RESULT_DUPLICATE_ROW", fmt.Sprintf("与第 %d 行重复", previousRow)))
		} else {
			dedupRows[dedupKey] = rowNumber
		}

		if len(rowErrors) > 0 {
			errorsByRow = append(errorsByRow, rowErrors...)
			continue
		}
		metrics := map[string]float64{}
		for name, value := range raw.Metrics {
			metrics[name] = value
		}
		var roi *float64
		if raw.Spend > 0 {
			value := raw.GMV / raw.Spend
			roi = &value
		}
		observations = append(observations, domain.PerformanceObservation{
			ID:              domain.NewID(),
			TenantID:        actor.TenantID,
			ProjectID:       in.ProjectID,
			RowNumber:       rowNumber,
			ScriptVersionID: script.ID,
			Platform:        strings.TrimSpace(raw.Platform),
			AccountAlias:    strings.TrimSpace(raw.AccountAlias),
			PublishedAt:     publishedAt,
			WindowHours:     raw.WindowHours,
			SampleStatus:    strings.TrimSpace(raw.SampleStatus),
			Metrics:         metrics,
			Currency:        currency,
			Spend:           raw.Spend,
			GMV:             raw.GMV,
			ROI:             roi,
			DedupKey:        dedupKey,
			IssueCategory:   strings.TrimSpace(raw.IssueCategory),
			Notes:           strings.TrimSpace(raw.Notes),
			CreatedAt:       now,
		})
	}

	keys := make([]string, 0, len(observations))
	for _, observation := range observations {
		keys = append(keys, observation.DedupKey)
	}
	if len(keys) > 0 {
		existing, err := s.store.ExistingPerformanceDedupKeys(ctx, actor.TenantID, in.ProjectID, keys)
		if err != nil {
			return result, err
		}
		for _, observation := range observations {
			if existingID := existing[observation.DedupKey]; existingID != "" {
				errorsByRow = append(errorsByRow, importRowError(observation.RowNumber, "", "RESULT_DUPLICATE_OBSERVATION", "相同剧本、平台、账号、发布时间和观察窗口的结果已存在"))
			}
		}
	}
	if len(errorsByRow) > 0 {
		sort.SliceStable(errorsByRow, func(i, j int) bool {
			if errorsByRow[i].RowNumber == errorsByRow[j].RowNumber {
				return errorsByRow[i].Field < errorsByRow[j].Field
			}
			return errorsByRow[i].RowNumber < errorsByRow[j].RowNumber
		})
		err := domain.Invalid("RESULT_IMPORT_REJECTED", "结果批次校验失败，未写入任何观察")
		err.Details = map[string]any{"row_count": len(in.Observations), "row_errors": errorsByRow}
		return result, err
	}

	sourceSHA := strings.ToLower(strings.TrimSpace(in.SourceSHA256))
	if sourceSHA == "" {
		var err error
		sourceSHA, err = domain.CanonicalHash(in.Observations)
		if err != nil {
			return result, err
		}
	}
	if !isLowerHexSHA256(sourceSHA) {
		return result, domain.Invalid("RESULT_SOURCE_HASH_INVALID", "结果来源 sha256 必须为 64 位小写十六进制")
	}
	batch := domain.PerformanceImportBatch{
		ID:            domain.NewID(),
		TenantID:      actor.TenantID,
		ProjectID:     in.ProjectID,
		SourceName:    sourceName,
		SourceFormat:  format,
		SourceSHA256:  sourceSHA,
		Currency:      batchCurrency,
		RowCount:      len(observations),
		ImportedCount: len(observations),
		Status:        "imported",
		CreatedBy:     actor.UserID,
		CreatedAt:     now,
	}
	for index := range observations {
		observations[index].ImportBatchID = batch.ID
	}
	if in.DryRun {
		batch.ID = ""
		batch.Status = "validated"
		for index := range observations {
			observations[index].ID = ""
			observations[index].ImportBatchID = ""
		}
		result.Batch, result.Observations = batch, observations
		return result, nil
	}
	if err := s.store.CreatePerformanceImportBatch(ctx, batch, observations); err != nil {
		return result, err
	}
	s.audit(ctx, actor, in.ProjectID, "performance_import.created", "performance_import_batch", batch.ID, requestID, map[string]any{"row_count": batch.RowCount, "currency": batch.Currency, "source_sha256": batch.SourceSHA256})
	result.Batch, result.Observations = batch, observations
	return result, nil
}

func validatePerformanceInput(rowNumber int, projectID string, in CreateObservationInput) []domain.PerformanceImportRowError {
	errors := []domain.PerformanceImportRowError{}
	add := func(field, code, message string) {
		errors = append(errors, importRowError(rowNumber, field, code, message))
	}
	if in.ProjectID != "" && in.ProjectID != projectID {
		add("project_id", "RESULT_PROJECT_MISMATCH", "行内项目与导入项目不一致")
	}
	if strings.TrimSpace(in.ScriptVersionID) == "" {
		add("script_version_id", "RESULT_FIELD_REQUIRED", "剧本版本必填")
	}
	for field, value := range map[string]string{"platform": in.Platform, "account_alias": in.AccountAlias, "sample_status": in.SampleStatus, "issue_category": in.IssueCategory, "notes": in.Notes} {
		if dangerousSpreadsheetText(value) {
			add(field, "RESULT_FORMULA_INJECTION", "文本不能以电子表格公式字符开头")
		}
	}
	if strings.TrimSpace(in.Platform) == "" {
		add("platform", "RESULT_FIELD_REQUIRED", "平台必填")
	}
	if strings.TrimSpace(in.AccountAlias) == "" {
		add("account_alias", "RESULT_FIELD_REQUIRED", "账号别名必填")
	}
	if in.PublishedAt.IsZero() {
		add("published_at", "RESULT_FIELD_REQUIRED", "发布时间必填")
	}
	if in.WindowHours <= 0 || in.WindowHours > 24*365 {
		add("window_hours", "RESULT_WINDOW_INVALID", "观察窗口必须在 1 到 8760 小时之间")
	}
	if !performanceSampleStatuses[strings.TrimSpace(in.SampleStatus)] {
		add("sample_status", "RESULT_SAMPLE_STATUS_INVALID", "样本状态无效")
	}
	if !performanceIssueCategories[strings.TrimSpace(in.IssueCategory)] {
		add("issue_category", "RESULT_ISSUE_CATEGORY_INVALID", "问题分类必须是 creative、account、delivery 或 landing")
	}
	for name, value := range in.Metrics {
		if !performanceMetrics[name] {
			add("metrics."+name, "RESULT_METRIC_UNSUPPORTED", "指标名称不受支持")
			continue
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			add("metrics."+name, "RESULT_METRIC_INVALID", "指标必须是非负有限数")
		} else if performanceRateMetrics[name] && value > 1 {
			add("metrics."+name, "RESULT_RATE_INVALID", "比例指标必须在 0 到 1 之间")
		}
	}
	if math.IsNaN(in.Spend) || math.IsInf(in.Spend, 0) || in.Spend < 0 {
		add("spend", "RESULT_SPEND_INVALID", "消耗必须是非负有限数")
	}
	if math.IsNaN(in.GMV) || math.IsInf(in.GMV, 0) || in.GMV < 0 {
		add("gmv", "RESULT_GMV_INVALID", "GMV 必须是非负有限数")
	}
	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency != "" && !isCurrencyCode(currency) {
		add("currency", "RESULT_CURRENCY_INVALID", "币种必须是 3 位大写 ISO 代码")
	}
	if (in.Spend > 0 || in.GMV > 0) && currency == "" {
		add("currency", "RESULT_CURRENCY_REQUIRED", "填写消耗或 GMV 时必须指定币种")
	}
	if in.SubmittedROI != nil && (math.IsNaN(*in.SubmittedROI) || math.IsInf(*in.SubmittedROI, 0)) {
		add("roi", "RESULT_ROI_INVALID", "ROI 必须是有限数；最终值仍由服务端计算")
	}
	return errors
}

func (s *Service) PerformanceObservations(ctx context.Context, actor Actor, projectID string) ([]domain.PerformanceObservation, error) {
	return s.store.PerformanceObservations(ctx, actor.TenantID, projectID)
}

func (s *Service) PerformanceImportBatches(ctx context.Context, actor Actor, projectID string) ([]domain.PerformanceImportBatch, error) {
	return s.store.PerformanceImportBatches(ctx, actor.TenantID, projectID)
}

func (s *Service) PerformanceImportDetails(ctx context.Context, actor Actor, id string) (domain.PerformanceImportDetails, error) {
	batch, err := s.store.PerformanceImportBatch(ctx, actor.TenantID, id)
	if err != nil {
		return domain.PerformanceImportDetails{}, err
	}
	all, err := s.store.PerformanceObservations(ctx, actor.TenantID, batch.ProjectID)
	if err != nil {
		return domain.PerformanceImportDetails{}, err
	}
	items := []domain.PerformanceObservation{}
	for _, observation := range all {
		if observation.ImportBatchID == batch.ID {
			items = append(items, observation)
		}
	}
	return domain.PerformanceImportDetails{Batch: batch, Observations: items}, nil
}

func (s *Service) CreateRatingDecision(ctx context.Context, actor Actor, in CreateRatingDecisionInput, requestID string) (CreateRatingDecisionResult, error) {
	result := CreateRatingDecisionResult{DryRun: in.DryRun}
	if actor.Type != "user" {
		return result, domain.Policy("USER_ACTOR_REQUIRED", "只有登录用户可以创建评级决策", "使用用户 CLI 凭据或 Web 会话")
	}
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return result, err
	}
	if _, err := s.projectForWrite(ctx, actor, in.ProjectID); err != nil {
		return result, err
	}
	if !performanceSampleStatuses[in.Rating] {
		return result, domain.Invalid("RATING_INVALID", "评级必须是 seed_candidate、repairable、discarded 或 insufficient_sample")
	}
	if strings.TrimSpace(in.Reason) == "" || strings.TrimSpace(in.NextAction) == "" {
		return result, domain.Invalid("RATING_REASON_REQUIRED", "评级依据和下一步动作必填")
	}
	if len(in.ObservationIDs) == 0 || len(in.ObservationIDs) > 100 {
		return result, domain.Invalid("RATING_OBSERVATIONS_INVALID", "评级必须引用 1 到 100 条结果观察")
	}
	if err := s.validateRatingSubject(ctx, actor, in.ProjectID, in.SubjectType, in.SubjectID); err != nil {
		return result, err
	}
	observationIDs := uniqueNonEmpty(in.ObservationIDs)
	if len(observationIDs) != len(in.ObservationIDs) {
		return result, domain.Invalid("RATING_OBSERVATIONS_DUPLICATE", "评级引用不能包含空值或重复观察")
	}
	for _, id := range observationIDs {
		observation, err := s.store.PerformanceObservation(ctx, actor.TenantID, id)
		if err != nil || observation.ProjectID != in.ProjectID {
			return result, domain.NotFound("结果观察")
		}
	}
	decision := domain.RatingDecision{
		ID:             domain.NewID(),
		TenantID:       actor.TenantID,
		ProjectID:      in.ProjectID,
		SubjectType:    in.SubjectType,
		SubjectID:      in.SubjectID,
		ObservationIDs: observationIDs,
		Rating:         in.Rating,
		Reason:         strings.TrimSpace(in.Reason),
		NextAction:     strings.TrimSpace(in.NextAction),
		CreatedBy:      actor.UserID,
		CreatedAt:      s.now().UTC(),
	}
	if in.DryRun {
		decision.ID = ""
		result.Decision = decision
		return result, nil
	}
	if err := s.store.CreateRatingDecision(ctx, decision); err != nil {
		return result, err
	}
	s.audit(ctx, actor, in.ProjectID, "rating_decision.created", "rating_decision", decision.ID, requestID, map[string]any{"subject_type": decision.SubjectType, "subject_id": decision.SubjectID, "rating": decision.Rating, "observation_count": len(decision.ObservationIDs)})
	result.Decision = decision
	return result, nil
}

func (s *Service) RatingDecisions(ctx context.Context, actor Actor, projectID string) ([]domain.RatingDecision, error) {
	return s.store.RatingDecisions(ctx, actor.TenantID, projectID)
}

func (s *Service) validateRatingSubject(ctx context.Context, actor Actor, projectID, subjectType, subjectID string) error {
	if strings.TrimSpace(subjectID) == "" {
		return domain.Invalid("RATING_SUBJECT_REQUIRED", "评级对象必填")
	}
	switch subjectType {
	case "script_version":
		v, err := s.store.Script(ctx, actor.TenantID, subjectID)
		if err != nil || v.ProjectID != projectID {
			return domain.NotFound("评级对象")
		}
	case "content_framework":
		v, err := s.store.Framework(ctx, actor.TenantID, subjectID)
		if err != nil || v.ProjectID != projectID {
			return domain.NotFound("评级对象")
		}
	case "shot_pattern":
		patterns, err := s.store.ShotPatterns(ctx, actor.TenantID, projectID)
		if err != nil {
			return err
		}
		for _, pattern := range patterns {
			if pattern.ID == subjectID {
				return nil
			}
		}
		return domain.NotFound("评级对象")
	default:
		return domain.Invalid("RATING_SUBJECT_TYPE_INVALID", "评级对象类型必须是 script_version、content_framework 或 shot_pattern")
	}
	return nil
}

func performanceDedupKey(projectID, scriptID, platform, account string, publishedAt time.Time, windowHours int) string {
	value := strings.Join([]string{projectID, strings.TrimSpace(scriptID), strings.ToLower(strings.TrimSpace(platform)), strings.ToLower(strings.TrimSpace(account)), publishedAt.UTC().Format(time.RFC3339Nano), fmt.Sprintf("%d", windowHours)}, "\n")
	return domain.TokenHash(value)
}

func importRowError(rowNumber int, field, code, message string) domain.PerformanceImportRowError {
	return domain.PerformanceImportRowError{RowNumber: rowNumber, Field: field, Code: code, Message: message}
}

func dangerousSpreadsheetText(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && strings.ContainsRune("=+@-", rune(value[0]))
}

func isCurrencyCode(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, char := range value {
		if char < 'A' || char > 'Z' {
			return false
		}
	}
	return true
}

func isLowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
