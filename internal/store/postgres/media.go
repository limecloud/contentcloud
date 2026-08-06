package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const taskStageOutputSelect = `SELECT tenant_id,id,project_id,task_id,stage_run_id,stage_id,output_type,object_id,object_version,object_digest,role,status,metadata,created_by,created_at FROM task_stage_outputs`

func scanTaskStageOutput(row pgx.Row) (domain.TaskStageOutput, error) {
	var value domain.TaskStageOutput
	var metadata []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.TaskID, &value.StageRunID, &value.StageID, &value.OutputType, &value.ObjectID, &value.ObjectVersion, &value.ObjectDigest, &value.Role, &value.Status, &metadata, &value.CreatedBy, &value.CreatedAt)
	if err == nil {
		value.Metadata, err = decodeJSON[map[string]any](metadata)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func insertTaskStageOutput(ctx context.Context, tx pgx.Tx, value domain.TaskStageOutput) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO task_stage_outputs(tenant_id,id,project_id,task_id,stage_run_id,stage_id,output_type,object_id,object_version,object_digest,role,status,metadata,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, value.TenantID, value.ID, value.ProjectID, value.TaskID, value.StageRunID, value.StageID, value.OutputType, value.ObjectID, value.ObjectVersion, value.ObjectDigest, value.Role, value.Status, jsonValue(value.Metadata), value.CreatedBy, value.CreatedAt)
	return dbError(err)
}

func (s *Store) CompleteStageRun(ctx context.Context, run domain.StageRun, outputs []domain.TaskStageOutput) error {
	return s.withTenant(ctx, run.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE stage_runs SET status=$3,execution_mode=$4,input_refs=$5,output_refs=$6,started_at=$7,completed_at=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2 AND task_id=$10`, run.TenantID, run.ID, run.Status, run.ExecutionMode, jsonArrayValue(run.InputRefs), jsonArrayValue(run.OutputRefs), run.StartedAt, run.CompletedAt, run.UpdatedAt, run.TaskID)
		if err != nil {
			return dbError(err)
		}
		if command.RowsAffected() == 0 {
			return domain.NotFound("阶段运行")
		}
		for _, output := range outputs {
			if output.TenantID != run.TenantID || output.TaskID != run.TaskID || output.StageRunID != run.ID || output.StageID != run.StageID {
				return domain.Invalid("TASK_STAGE_OUTPUT_SCOPE_INVALID", "阶段输出与阶段执行记录的作用域不一致")
			}
			if err := insertTaskStageOutput(ctx, tx, output); err != nil {
				return err
			}
		}
		return nil
	})
}

func taskStageOutputsTx(ctx context.Context, tx pgx.Tx, tenantID, taskID string) ([]domain.TaskStageOutput, error) {
	rows, err := tx.Query(ctx, taskStageOutputSelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY created_at,id`, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.TaskStageOutput{}
	for rows.Next() {
		value, scanErr := scanTaskStageOutput(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) TaskStageOutputs(ctx context.Context, tenantID, taskID string) ([]domain.TaskStageOutput, error) {
	result := []domain.TaskStageOutput{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var err error
		result, err = taskStageOutputsTx(ctx, tx, tenantID, taskID)
		return err
	})
	return result, err
}

func (s *Store) CreateProviderProfile(ctx context.Context, value domain.ProviderProfile) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO provider_profiles(provider_id,version,digest,adapter_version,model,region,modes,input_media_types,output_media_type,limits,data_retention,pricing,status,verified_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, value.ProviderID, value.Version, value.Digest, value.AdapterVersion, value.Model, value.Region, jsonArrayValue(value.Modes), jsonArrayValue(value.InputMediaTypes), value.OutputMediaType, jsonValue(value.Limits), value.DataRetention, jsonValue(value.Pricing), value.Status, value.VerifiedAt, value.ExpiresAt)
	return dbError(err)
}

func scanProviderProfile(row pgx.Row) (domain.ProviderProfile, error) {
	var value domain.ProviderProfile
	var modes, inputMediaTypes, limits, pricing []byte
	err := row.Scan(&value.ProviderID, &value.Version, &value.Digest, &value.AdapterVersion, &value.Model, &value.Region, &modes, &inputMediaTypes, &value.OutputMediaType, &limits, &value.DataRetention, &pricing, &value.Status, &value.VerifiedAt, &value.ExpiresAt)
	if err == nil {
		value.Modes, err = decodeJSON[[]string](modes)
	}
	if err == nil {
		value.InputMediaTypes, err = decodeJSON[[]string](inputMediaTypes)
	}
	if err == nil {
		value.Limits, err = decodeJSON[map[string]any](limits)
	}
	if err == nil {
		value.Pricing, err = decodeJSON[map[string]any](pricing)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) ProviderProfile(ctx context.Context, providerID, version string) (domain.ProviderProfile, error) {
	value, err := scanProviderProfile(s.pool.QueryRow(ctx, `SELECT provider_id,version,digest,adapter_version,model,region,modes,input_media_types,output_media_type,limits,data_retention,pricing,status,verified_at,expires_at FROM provider_profiles WHERE provider_id=$1 AND version=$2`, providerID, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return value, domain.NotFound("服务商配置")
	}
	return value, err
}

func (s *Store) SaveProviderBinding(ctx context.Context, value domain.ProviderBinding) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO provider_bindings(tenant_id,provider_id,profile_version,state,credential_ref,egress_policy,monthly_budget_minor,max_job_cost_minor,max_concurrency,max_retries,updated_by,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (tenant_id,provider_id) DO UPDATE SET profile_version=EXCLUDED.profile_version,state=EXCLUDED.state,credential_ref=EXCLUDED.credential_ref,egress_policy=EXCLUDED.egress_policy,monthly_budget_minor=EXCLUDED.monthly_budget_minor,max_job_cost_minor=EXCLUDED.max_job_cost_minor,max_concurrency=EXCLUDED.max_concurrency,max_retries=EXCLUDED.max_retries,updated_by=EXCLUDED.updated_by,updated_at=EXCLUDED.updated_at`, value.TenantID, value.ProviderID, value.ProfileVersion, value.State, value.CredentialRef, value.EgressPolicy, value.MonthlyBudgetMinor, value.MaxJobCostMinor, value.MaxConcurrency, value.MaxRetries, value.UpdatedBy, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) ProviderBinding(ctx context.Context, tenantID, providerID string) (domain.ProviderBinding, error) {
	var result domain.ProviderBinding
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT tenant_id,provider_id,profile_version,state,credential_ref,egress_policy,monthly_budget_minor,max_job_cost_minor,max_concurrency,max_retries,updated_by,updated_at FROM provider_bindings WHERE tenant_id=$1 AND provider_id=$2`, tenantID, providerID).Scan(&result.TenantID, &result.ProviderID, &result.ProfileVersion, &result.State, &result.CredentialRef, &result.EgressPolicy, &result.MonthlyBudgetMinor, &result.MaxJobCostMinor, &result.MaxConcurrency, &result.MaxRetries, &result.UpdatedBy, &result.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("服务商绑定")
		}
		return dbError(err)
	})
	return result, err
}

const mediaGenerationJobSelect = `SELECT tenant_id,id,project_id,task_id,stage_run_id,storyboard_snapshot_id,prompt_package_artifact_id,provider_id,profile_version,profile_digest,model,mode,aspect_ratio,duration_seconds,input_artifact_refs,state,idempotency_key,estimated_cost_minor,actual_cost_minor,currency,attempt_count,max_attempts,lease_owner,lease_expires_at,cancel_requested_at,error_code,error_detail_safe,row_version,created_by,created_at,updated_at FROM media_generation_jobs`

func scanMediaGenerationJob(row pgx.Row) (domain.MediaGenerationJob, error) {
	var value domain.MediaGenerationJob
	var inputs []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.TaskID, &value.StageRunID, &value.StoryboardSnapshotID, &value.PromptPackageArtifactID, &value.ProviderID, &value.ProfileVersion, &value.ProfileDigest, &value.Model, &value.Mode, &value.AspectRatio, &value.DurationSeconds, &inputs, &value.State, &value.IdempotencyKey, &value.EstimatedCostMinor, &value.ActualCostMinor, &value.Currency, &value.AttemptCount, &value.MaxAttempts, &value.LeaseOwner, &value.LeaseExpiresAt, &value.CancelRequestedAt, &value.ErrorCode, &value.ErrorDetailSafe, &value.RowVersion, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.InputArtifactRefs, err = decodeJSON[[]string](inputs)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateMediaGenerationJob(ctx context.Context, value domain.MediaGenerationJob) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO media_generation_jobs(tenant_id,id,project_id,task_id,stage_run_id,storyboard_snapshot_id,prompt_package_artifact_id,provider_id,profile_version,profile_digest,model,mode,aspect_ratio,duration_seconds,input_artifact_refs,state,idempotency_key,estimated_cost_minor,actual_cost_minor,currency,attempt_count,max_attempts,lease_owner,lease_expires_at,cancel_requested_at,error_code,error_detail_safe,row_version,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31)`, value.TenantID, value.ID, value.ProjectID, value.TaskID, value.StageRunID, value.StoryboardSnapshotID, value.PromptPackageArtifactID, value.ProviderID, value.ProfileVersion, value.ProfileDigest, value.Model, value.Mode, value.AspectRatio, value.DurationSeconds, jsonArrayValue(value.InputArtifactRefs), value.State, value.IdempotencyKey, value.EstimatedCostMinor, value.ActualCostMinor, value.Currency, value.AttemptCount, value.MaxAttempts, value.LeaseOwner, value.LeaseExpiresAt, value.CancelRequestedAt, value.ErrorCode, value.ErrorDetailSafe, value.RowVersion, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) PendingMediaGenerationJobs(ctx context.Context, limit int) ([]domain.MediaGenerationJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `SELECT tenant_id,job_id FROM contentcloud_pending_media_generation_jobs($1)`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type ref struct{ tenantID, jobID string }
	refs := []ref{}
	for rows.Next() {
		var value ref
		if err := rows.Scan(&value.tenantID, &value.jobID); err != nil {
			return nil, err
		}
		refs = append(refs, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]domain.MediaGenerationJob, 0, len(refs))
	for _, value := range refs {
		job, err := s.MediaGenerationJob(ctx, value.tenantID, value.jobID)
		if err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, nil
}

func (s *Store) MediaGenerationJob(ctx context.Context, tenantID, id string) (domain.MediaGenerationJob, error) {
	var result domain.MediaGenerationJob
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanMediaGenerationJob(tx.QueryRow(ctx, mediaGenerationJobSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("媒体生成任务")
		}
		return err
	})
	return result, err
}

func (s *Store) MediaGenerationJobs(ctx context.Context, tenantID, taskID string) ([]domain.MediaGenerationJob, error) {
	result := []domain.MediaGenerationJob{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, mediaGenerationJobSelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY created_at,id`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanMediaGenerationJob(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) SaveMediaGenerationJob(ctx context.Context, value domain.MediaGenerationJob, expectedVersion int) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		var currentState string
		var currentVersion int
		err := tx.QueryRow(ctx, `SELECT state,row_version FROM media_generation_jobs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, value.TenantID, value.ID).Scan(&currentState, &currentVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("媒体生成任务")
		}
		if err != nil {
			return dbError(err)
		}
		if currentVersion != expectedVersion {
			return domain.Conflict("MEDIA_JOB_STALE", "媒体生成任务已被其他操作更新")
		}
		if !domain.CanTransitionMediaJob(currentState, value.State) {
			return domain.Conflict("MEDIA_JOB_TRANSITION_INVALID", "媒体生成任务状态转换无效")
		}
		value.RowVersion = expectedVersion + 1
		_, err = tx.Exec(ctx, `UPDATE media_generation_jobs SET state=$3,estimated_cost_minor=$4,actual_cost_minor=$5,attempt_count=$6,lease_owner=$7,lease_expires_at=$8,cancel_requested_at=$9,error_code=$10,error_detail_safe=$11,row_version=$12,updated_at=$13 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.State, value.EstimatedCostMinor, value.ActualCostMinor, value.AttemptCount, value.LeaseOwner, value.LeaseExpiresAt, value.CancelRequestedAt, value.ErrorCode, value.ErrorDetailSafe, value.RowVersion, value.UpdatedAt)
		return dbError(err)
	})
}

const providerAttemptSelect = `SELECT tenant_id,id,project_id,generation_job_id,attempt_number,provider_id,request_digest,external_job_id,provider_state,safe_request_summary,safe_response_summary,disclosure_manifest,http_status,provider_request_id,estimated_cost_minor,actual_cost_minor,currency,last_polled_at,next_poll_at,submitted_at,downloaded_at,completed_at,retry_after_seconds,error_code,error_detail_safe,created_at,updated_at FROM provider_attempts`

func scanProviderAttempt(row pgx.Row) (domain.ProviderAttempt, error) {
	var value domain.ProviderAttempt
	var requestSummary, responseSummary, disclosure []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.GenerationJobID, &value.AttemptNumber, &value.ProviderID, &value.RequestDigest, &value.ExternalJobID, &value.ProviderState, &requestSummary, &responseSummary, &disclosure, &value.HTTPStatus, &value.ProviderRequestID, &value.EstimatedCostMinor, &value.ActualCostMinor, &value.Currency, &value.LastPolledAt, &value.NextPollAt, &value.SubmittedAt, &value.DownloadedAt, &value.CompletedAt, &value.RetryAfterSeconds, &value.ErrorCode, &value.ErrorDetailSafe, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.SafeRequestSummary, err = decodeJSON[map[string]any](requestSummary)
	}
	if err == nil {
		value.SafeResponseSummary, err = decodeJSON[map[string]any](responseSummary)
	}
	if err == nil {
		value.DisclosureManifest, err = decodeJSON[map[string]any](disclosure)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateProviderAttempt(ctx context.Context, value domain.ProviderAttempt) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO provider_attempts(tenant_id,id,project_id,generation_job_id,attempt_number,provider_id,request_digest,external_job_id,provider_state,safe_request_summary,safe_response_summary,disclosure_manifest,http_status,provider_request_id,estimated_cost_minor,actual_cost_minor,currency,last_polled_at,next_poll_at,submitted_at,downloaded_at,completed_at,retry_after_seconds,error_code,error_detail_safe,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)`, value.TenantID, value.ID, value.ProjectID, value.GenerationJobID, value.AttemptNumber, value.ProviderID, value.RequestDigest, value.ExternalJobID, value.ProviderState, jsonValue(value.SafeRequestSummary), jsonValue(value.SafeResponseSummary), jsonValue(value.DisclosureManifest), value.HTTPStatus, value.ProviderRequestID, value.EstimatedCostMinor, value.ActualCostMinor, value.Currency, value.LastPolledAt, value.NextPollAt, value.SubmittedAt, value.DownloadedAt, value.CompletedAt, value.RetryAfterSeconds, value.ErrorCode, value.ErrorDetailSafe, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) SaveProviderAttempt(ctx context.Context, value domain.ProviderAttempt) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE provider_attempts SET external_job_id=$3,provider_state=$4,safe_response_summary=$5,disclosure_manifest=$6,http_status=$7,provider_request_id=$8,actual_cost_minor=$9,last_polled_at=$10,next_poll_at=$11,submitted_at=$12,downloaded_at=$13,completed_at=$14,retry_after_seconds=$15,error_code=$16,error_detail_safe=$17,updated_at=$18 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.ExternalJobID, value.ProviderState, jsonValue(value.SafeResponseSummary), jsonValue(value.DisclosureManifest), value.HTTPStatus, value.ProviderRequestID, value.ActualCostMinor, value.LastPolledAt, value.NextPollAt, value.SubmittedAt, value.DownloadedAt, value.CompletedAt, value.RetryAfterSeconds, value.ErrorCode, value.ErrorDetailSafe, value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return domain.NotFound("服务商执行尝试")
		}
		return dbError(err)
	})
}

func (s *Store) ProviderAttempts(ctx context.Context, tenantID, jobID string) ([]domain.ProviderAttempt, error) {
	result := []domain.ProviderAttempt{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, providerAttemptSelect+` WHERE tenant_id=$1 AND generation_job_id=$2 ORDER BY attempt_number`, tenantID, jobID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanProviderAttempt(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

const mediaReviewSelect = `SELECT tenant_id,id,project_id,task_id,generation_job_id,subject_artifact_id,subject_digest,review_kind,status,checks,selected,decision_reason,decided_by,decided_at,row_version,created_by,created_at,updated_at FROM media_reviews`

func scanMediaReview(row pgx.Row) (domain.MediaReview, error) {
	var value domain.MediaReview
	var checks []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.TaskID, &value.GenerationJobID, &value.SubjectArtifactID, &value.SubjectDigest, &value.ReviewKind, &value.Status, &checks, &value.Selected, &value.DecisionReason, &value.DecidedBy, &value.DecidedAt, &value.RowVersion, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.Checks, err = decodeJSON[map[string]any](checks)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateMediaReview(ctx context.Context, value domain.MediaReview) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO media_reviews(tenant_id,id,project_id,task_id,generation_job_id,subject_artifact_id,subject_digest,review_kind,status,checks,selected,decision_reason,decided_by,decided_at,row_version,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, value.TenantID, value.ID, value.ProjectID, value.TaskID, value.GenerationJobID, value.SubjectArtifactID, value.SubjectDigest, value.ReviewKind, value.Status, jsonValue(value.Checks), value.Selected, value.DecisionReason, value.DecidedBy, value.DecidedAt, value.RowVersion, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) SaveMediaReview(ctx context.Context, value domain.MediaReview, expectedVersion int) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		value.RowVersion = expectedVersion + 1
		command, err := tx.Exec(ctx, `UPDATE media_reviews SET status=$3,checks=$4,selected=$5,decision_reason=$6,decided_by=$7,decided_at=$8,row_version=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2 AND row_version=$11`, value.TenantID, value.ID, value.Status, jsonValue(value.Checks), value.Selected, value.DecisionReason, value.DecidedBy, value.DecidedAt, value.RowVersion, value.UpdatedAt, expectedVersion)
		if err == nil && command.RowsAffected() == 0 {
			return domain.Conflict("MEDIA_REVIEW_STALE", "媒体审核已被其他操作更新")
		}
		return dbError(err)
	})
}

func (s *Store) MediaReview(ctx context.Context, tenantID, id string) (domain.MediaReview, error) {
	var result domain.MediaReview
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanMediaReview(tx.QueryRow(ctx, mediaReviewSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("媒体审核")
		}
		return err
	})
	return result, err
}

func (s *Store) MediaReviews(ctx context.Context, tenantID, taskID string) ([]domain.MediaReview, error) {
	result := []domain.MediaReview{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, mediaReviewSelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY created_at,id`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanMediaReview(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}
