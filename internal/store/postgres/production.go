package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	storecontract "github.com/limecloud/contentcloud/internal/store"
)

func (s *Store) CreateBrief(ctx context.Context, v domain.BriefVersion) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO brief_versions(id,tenant_id,project_id,version,status,payload,content_hash,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, v.TenantID, v.ProjectID, v.Version, v.Status, jsonValue(v), v.ContentHash, v.CreatedAt)
		return dbError(err)
	})
}

func scanBrief(row pgx.Row) (domain.BriefVersion, error) {
	var body []byte
	var status, contentHash string
	err := row.Scan(&body, &status, &contentHash)
	if err != nil {
		return domain.BriefVersion{}, err
	}
	v, err := decodeJSON[domain.BriefVersion](body)
	v.Status, v.ContentHash = status, contentHash
	return v, err
}

func (s *Store) Briefs(ctx context.Context, tenantID, projectID string) ([]domain.BriefVersion, error) {
	out := []domain.BriefVersion{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT payload,status,content_hash FROM brief_versions WHERE tenant_id=$1 AND project_id=$2 ORDER BY version DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanBrief(rows)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) Brief(ctx context.Context, tenantID, id string) (domain.BriefVersion, error) {
	var result domain.BriefVersion
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanBrief(tx.QueryRow(ctx, `SELECT payload,status,content_hash FROM brief_versions WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Brief")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveBrief(ctx context.Context, v domain.BriefVersion) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE brief_versions SET status=$3,payload=$4,content_hash=$5 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.Status, jsonValue(v), v.ContentHash)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("Brief")
		}
		return nil
	})
}

func (s *Store) CreateSnapshot(ctx context.Context, v domain.ContextSnapshot) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO context_snapshots(id,tenant_id,project_id,brief_version_id,builder_version,schema_version,payload,manifest_hash,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, v.ID, v.TenantID, v.ProjectID, nullable(v.BriefVersionID), v.BuilderVersion, v.SchemaVersion, jsonValue(v), v.ManifestHash, v.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) Snapshot(ctx context.Context, tenantID, id string) (domain.ContextSnapshot, error) {
	var result domain.ContextSnapshot
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var body []byte
		if err := tx.QueryRow(ctx, `SELECT payload FROM context_snapshots WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&body); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("上下文快照")
			}
			return err
		}
		v, err := decodeJSON[domain.ContextSnapshot](body)
		result = v
		return err
	})
	return result, err
}

func (s *Store) CreateRun(ctx context.Context, v domain.TaskRun) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		return insertRun(ctx, tx, v)
	})
}

func (s *Store) CreateRunWithBundle(ctx context.Context, v domain.TaskRun, bundle environment.CreativeExecutionBundle) error {
	if bundle.ProjectID != v.ProjectID || bundle.Subject.ID != v.InputSnapshotID {
		return domain.Conflict("EXECUTION_BUNDLE_RUN_MISMATCH", "CreativeExecutionBundle 与 TaskRun 不匹配")
	}
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		if err := insertRun(ctx, tx, v); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO creative_execution_bundles(run_id,tenant_id,project_id,bundle_id,digest,payload,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, v.ID, v.TenantID, v.ProjectID, bundle.BundleID, bundle.Digest, jsonValue(bundle), bundle.IssuedAt)
		return dbError(err)
	})
}

func insertRun(ctx context.Context, tx pgx.Tx, v domain.TaskRun) error {
	_, err := tx.Exec(ctx, `INSERT INTO task_runs(id,tenant_id,project_id,brief_version_id,input_snapshot_id,idempotency_key,task_type,capability_id,capability_version,input_schema,output_schema,output_count,delivery_profiles,script_id,baseline_script_version_id,change_type,invariant_fields,expected_changed_fields,hypothesis,revision_reason,state,priority,attempt_count,active_attempt_id,lease_device_id,lease_expires_at,run_token_hash,progress_label,error_code,cancel_requested_at,report_hash,heartbeat_sequence,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34)`, v.ID, v.TenantID, v.ProjectID, nullable(v.BriefVersionID), v.InputSnapshotID, v.IdempotencyKey, v.TaskType, v.CapabilityID, v.CapabilityVersion, v.InputSchema, v.OutputSchema, v.OutputCount, jsonValue(v.DeliveryProfiles), nullable(v.ScriptID), nullable(v.BaselineVersionID), v.ChangeType, jsonValue(v.InvariantFields), jsonValue(v.ExpectedChanges), v.Hypothesis, v.RevisionReason, v.State, v.Priority, v.AttemptCount, nullable(v.ActiveAttemptID), nullable(v.LeaseDeviceID), v.LeaseExpiresAt, v.RunTokenHash, v.ProgressLabel, v.ErrorCode, v.CancelRequestedAt, v.ReportHash, v.HeartbeatSequence, v.CreatedAt, v.UpdatedAt)
	return dbError(err)
}

func (s *Store) ExecutionBundle(ctx context.Context, tenantID, runID string) (environment.CreativeExecutionBundle, error) {
	var result environment.CreativeExecutionBundle
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var body []byte
		if err := tx.QueryRow(ctx, `SELECT payload FROM creative_execution_bundles WHERE tenant_id=$1 AND run_id=$2`, tenantID, runID).Scan(&body); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("CreativeExecutionBundle")
			}
			return err
		}
		bundle, err := decodeJSON[environment.CreativeExecutionBundle](body)
		result = bundle
		return err
	})
	return result, err
}

func scanRun(row pgx.Row) (domain.TaskRun, error) {
	var v domain.TaskRun
	var profiles, invariantFields, expectedChanges []byte
	err := row.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.BriefVersionID, &v.InputSnapshotID, &v.IdempotencyKey, &v.TaskType, &v.CapabilityID, &v.CapabilityVersion, &v.InputSchema, &v.OutputSchema, &v.OutputCount, &profiles, &v.ScriptID, &v.BaselineVersionID, &v.ChangeType, &invariantFields, &expectedChanges, &v.Hypothesis, &v.RevisionReason, &v.State, &v.Priority, &v.AttemptCount, &v.ActiveAttemptID, &v.LeaseDeviceID, &v.LeaseExpiresAt, &v.RunTokenHash, &v.ProgressLabel, &v.ErrorCode, &v.CancelRequestedAt, &v.ReportHash, &v.HeartbeatSequence, &v.CreatedAt, &v.UpdatedAt)
	if err == nil {
		v.DeliveryProfiles, err = decodeJSON[[]string](profiles)
	}
	if err == nil {
		v.InvariantFields, err = decodeJSON[[]string](invariantFields)
	}
	if err == nil {
		v.ExpectedChanges, err = decodeJSON[[]string](expectedChanges)
	}
	return v, err
}

const runSelect = `SELECT id,tenant_id,project_id,COALESCE(brief_version_id::text,''),input_snapshot_id,idempotency_key,task_type,capability_id,capability_version,input_schema,output_schema,output_count,delivery_profiles,COALESCE(script_id::text,''),COALESCE(baseline_script_version_id::text,''),change_type,invariant_fields,expected_changed_fields,hypothesis,revision_reason,state,priority,attempt_count,COALESCE(active_attempt_id::text,''),COALESCE(lease_device_id::text,''),lease_expires_at,run_token_hash,COALESCE(progress_label,''),COALESCE(error_code,''),cancel_requested_at,report_hash,heartbeat_sequence,created_at,updated_at FROM task_runs`

func (s *Store) Runs(ctx context.Context, tenantID, projectID string) ([]domain.TaskRun, error) {
	out := []domain.TaskRun{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if projectID != "" {
			query += ` AND project_id=$2`
			args = append(args, projectID)
		}
		query += ` ORDER BY created_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanRun(rows)
			if err != nil {
				return err
			}
			v.RunTokenHash = ""
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) Run(ctx context.Context, tenantID, id string) (domain.TaskRun, error) {
	var result domain.TaskRun
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanRun(tx.QueryRow(ctx, runSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("任务")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveRun(ctx context.Context, v domain.TaskRun) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE task_runs SET state=$3,priority=$4,attempt_count=$5,active_attempt_id=$6,lease_device_id=$7,lease_expires_at=$8,run_token_hash=$9,progress_label=$10,error_code=$11,cancel_requested_at=$12,report_hash=$13,heartbeat_sequence=$14,script_id=$15,updated_at=$16 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.State, v.Priority, v.AttemptCount, nullable(v.ActiveAttemptID), nullable(v.LeaseDeviceID), v.LeaseExpiresAt, v.RunTokenHash, v.ProgressLabel, v.ErrorCode, v.CancelRequestedAt, v.ReportHash, v.HeartbeatSequence, nullable(v.ScriptID), v.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("任务")
		}
		return nil
	})
}

func (s *Store) LeaseNextRun(ctx context.Context, tenantID, deviceID string, eligible []storecontract.RunLeaseCandidate, attemptID, tokenHash string, now time.Time) (domain.TaskRun, domain.RunAttempt, error) {
	if len(eligible) == 0 {
		return domain.TaskRun{}, domain.RunAttempt{}, domain.NotFound("可领取任务")
	}
	capabilities := make(map[string]domain.Capability, len(eligible))
	for _, candidate := range eligible {
		if candidate.RunID != "" && candidate.Capability.ID != "" {
			capabilities[candidate.RunID] = candidate.Capability
		}
	}
	var result domain.TaskRun
	var leasedAttempt domain.RunAttempt
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices d JOIN project_device_grants g ON g.device_id=d.id WHERE d.tenant_id=$1 AND d.id=$2 AND d.revoked_at IS NULL AND g.revoked_at IS NULL)`, tenantID, deviceID).Scan(&allowed); err != nil || !allowed {
			return domain.NotFound("设备")
		}
		rows, err := tx.Query(ctx, runSelect+` WHERE tenant_id=$1 AND state='queued' AND attempt_count<3 AND EXISTS(SELECT 1 FROM project_device_grants g WHERE g.project_id=task_runs.project_id AND g.device_id=$2 AND g.revoked_at IS NULL) ORDER BY priority DESC,created_at FOR UPDATE SKIP LOCKED LIMIT 100`, tenantID, deviceID)
		if err != nil {
			return err
		}
		var candidates []domain.TaskRun
		for rows.Next() {
			candidate, scanErr := scanRun(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			candidates = append(candidates, candidate)
		}
		rows.Close()
		var v domain.TaskRun
		var capability domain.Capability
		matched := false
		for _, candidate := range candidates {
			candidateCapability, allowed := capabilities[candidate.ID]
			if allowed && candidate.AcceptsCapability(candidateCapability) {
				v, matched = candidate, true
				capability = candidateCapability
				break
			}
		}
		if !matched {
			return domain.NotFound("可领取任务")
		}
		until := now.Add(5 * time.Minute)
		v.State = "leased"
		v.LeaseDeviceID = deviceID
		v.LeaseExpiresAt = &until
		v.AttemptCount++
		v.UpdatedAt = now
		attempt := domain.RunAttempt{ID: attemptID, TenantID: tenantID, ProjectID: v.ProjectID, RunID: v.ID, DeviceID: deviceID, State: "leased", CapabilityID: capability.ID, CapabilityVersion: capability.Version, CapabilityDigest: capability.Digest, InputSchema: capability.InputSchema, OutputSchema: capability.OutputSchema, TokenHash: tokenHash, LeaseExpiresAt: until, Usage: map[string]any{}, CreatedAt: now}
		if _, err := tx.Exec(ctx, `INSERT INTO run_attempts(id,tenant_id,project_id,run_id,device_id,state,capability_id,capability_version,capability_digest,input_schema,output_schema,token_hash,lease_expires_at,usage,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, attempt.ID, attempt.TenantID, attempt.ProjectID, attempt.RunID, attempt.DeviceID, attempt.State, attempt.CapabilityID, attempt.CapabilityVersion, attempt.CapabilityDigest, attempt.InputSchema, attempt.OutputSchema, attempt.TokenHash, attempt.LeaseExpiresAt, jsonValue(attempt.Usage), attempt.CreatedAt); err != nil {
			return err
		}
		v.ActiveAttemptID = attempt.ID
		v.RunTokenHash = tokenHash
		v.ProgressLabel = "任务已领取"
		if _, err := tx.Exec(ctx, `UPDATE task_runs SET state=$3,lease_device_id=$4,lease_expires_at=$5,attempt_count=$6,active_attempt_id=$7,run_token_hash=$8,progress_label=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2`, tenantID, v.ID, v.State, deviceID, until, v.AttemptCount, attempt.ID, tokenHash, v.ProgressLabel, now); err != nil {
			return err
		}
		result = v
		leasedAttempt = attempt
		return nil
	})
	return result, leasedAttempt, err
}

func (s *Store) CreateScript(ctx context.Context, script domain.Script, version domain.ScriptVersion) (domain.ScriptVersion, error) {
	version.ScriptID = script.ID
	version.Version = 1
	err := s.withTenant(ctx, version.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO scripts(id,tenant_id,project_id,title,created_at) VALUES($1,$2,$3,$4,$5)`, script.ID, script.TenantID, script.ProjectID, script.Title, script.CreatedAt); err != nil {
			return dbError(err)
		}
		return insertScriptVersion(ctx, tx, version)
	})
	return version, err
}

func (s *Store) CreateScriptVersion(ctx context.Context, version domain.ScriptVersion) (domain.ScriptVersion, error) {
	err := s.withTenant(ctx, version.TenantID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT true FROM scripts WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, version.TenantID, version.ScriptID).Scan(&exists); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("逻辑剧本")
			}
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM script_versions WHERE tenant_id=$1 AND script_id=$2`, version.TenantID, version.ScriptID).Scan(&version.Version); err != nil {
			return err
		}
		return insertScriptVersion(ctx, tx, version)
	})
	return version, err
}

func insertScriptVersion(ctx context.Context, tx pgx.Tx, v domain.ScriptVersion) error {
	_, err := tx.Exec(ctx, `INSERT INTO script_versions(id,tenant_id,project_id,script_id,run_id,version,supersedes_id,baseline_script_version_id,change_type,invariant_fields,changed_fields,hypothesis,revision_reason,status,input_snapshot_id,content_hash,canonical_json,validation_report,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, v.ID, v.TenantID, v.ProjectID, v.ScriptID, v.RunID, v.Version, nullable(v.SupersedesID), nullable(v.BaselineID), v.ChangeType, jsonValue(v.InvariantFields), jsonValue(v.ChangedFields), v.Hypothesis, v.RevisionReason, v.Status, v.InputSnapshotID, v.ContentHash, jsonValue(v.Package), jsonValue(v.Validation), v.CreatedAt)
	return dbError(err)
}
func scanScript(row pgx.Row) (domain.ScriptVersion, error) {
	var v domain.ScriptVersion
	var invariantFields, changedFields, pkg, validation []byte
	err := row.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.ScriptID, &v.RunID, &v.Version, &v.SupersedesID, &v.BaselineID, &v.ChangeType, &invariantFields, &changedFields, &v.Hypothesis, &v.RevisionReason, &v.Status, &v.InputSnapshotID, &v.ContentHash, &pkg, &validation, &v.CreatedAt)
	if err == nil {
		v.InvariantFields, err = decodeJSON[[]string](invariantFields)
	}
	if err == nil {
		v.ChangedFields, err = decodeJSON[[]string](changedFields)
	}
	if err == nil {
		v.Package, err = decodeJSON[domain.ScriptPackage](pkg)
	}
	if err == nil {
		v.Validation, err = decodeJSON[domain.ValidationReport](validation)
	}
	return v, err
}

const scriptSelect = `SELECT id,tenant_id,project_id,script_id,run_id,version,COALESCE(supersedes_id::text,''),COALESCE(baseline_script_version_id::text,''),change_type,invariant_fields,changed_fields,hypothesis,revision_reason,status,input_snapshot_id,content_hash,canonical_json,validation_report,created_at FROM script_versions`

func (s *Store) Scripts(ctx context.Context, tenantID, projectID string) ([]domain.ScriptVersion, error) {
	out := []domain.ScriptVersion{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, scriptSelect+` WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanScript(rows)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}
func (s *Store) Script(ctx context.Context, tenantID, id string) (domain.ScriptVersion, error) {
	var result domain.ScriptVersion
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanScript(tx.QueryRow(ctx, scriptSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("剧本")
		}
		return err
	})
	return result, err
}
func (s *Store) SaveScript(ctx context.Context, v domain.ScriptVersion) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE script_versions SET status=$3 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.Status)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("剧本")
		}
		return nil
	})
}

func (s *Store) CreateApproval(ctx context.Context, v domain.ApprovalDecision) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO approval_decisions(id,tenant_id,project_id,subject_type,subject_id,subject_hash,decision_stage,actor_id,decision,reason,previous_state,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, v.ID, v.TenantID, v.ProjectID, v.SubjectType, v.SubjectID, v.SubjectHash, defaultDecisionStage(v.DecisionStage), v.ActorID, v.Decision, v.Reason, v.PreviousState, v.ResultingState, v.CreatedAt)
		return dbError(err)
	})
}
func (s *Store) Approvals(ctx context.Context, tenantID, subjectID string) ([]domain.ApprovalDecision, error) {
	out := []domain.ApprovalDecision{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := `SELECT id,tenant_id,project_id,subject_type,subject_id,subject_hash,decision_stage,actor_id,decision,reason,previous_state,resulting_state,created_at FROM approval_decisions WHERE tenant_id=$1`
		args := []any{tenantID}
		if subjectID != "" {
			query += ` AND subject_id=$2`
			args = append(args, subjectID)
		}
		query += ` ORDER BY created_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.ApprovalDecision
			if err := rows.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.SubjectType, &v.SubjectID, &v.SubjectHash, &v.DecisionStage, &v.ActorID, &v.Decision, &v.Reason, &v.PreviousState, &v.ResultingState, &v.CreatedAt); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func defaultDecisionStage(value string) string {
	if value == "" {
		return "legacy"
	}
	return value
}

func (s *Store) AppendAudit(ctx context.Context, v domain.AuditEvent) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,tenant_id,project_id,actor_type,actor_id,action,subject_type,subject_id,summary,request_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.ID, v.TenantID, nullable(v.ProjectID), v.ActorType, v.ActorID, v.Action, v.SubjectType, v.SubjectID, jsonValue(v.Summary), v.RequestID, v.CreatedAt)
		return dbError(err)
	})
}
func (s *Store) AuditEvents(ctx context.Context, tenantID, projectID string, limit int) ([]domain.AuditEvent, error) {
	out := []domain.AuditEvent{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := `SELECT id,tenant_id,COALESCE(project_id::text,''),actor_type,actor_id,action,subject_type,subject_id,summary,request_id,created_at FROM audit_events WHERE tenant_id=$1`
		args := []any{tenantID}
		if projectID != "" {
			query += ` AND project_id=$2`
			args = append(args, projectID)
		}
		query += ` ORDER BY created_at DESC LIMIT $` + itoa(len(args)+1)
		args = append(args, limit)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.AuditEvent
			var summary []byte
			if err := rows.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.ActorType, &v.ActorID, &v.Action, &v.SubjectType, &v.SubjectID, &summary, &v.RequestID, &v.CreatedAt); err != nil {
				return err
			}
			v.Summary, err = decodeJSON[map[string]any](summary)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func itoa(value int) string {
	if value < 10 {
		return string(rune('0' + value))
	}
	return "10"
}
