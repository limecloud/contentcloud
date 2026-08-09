package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateApproval(ctx context.Context, value domain.ApprovalDecision) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO approval_decisions(id,tenant_id,project_id,subject_type,subject_id,subject_hash,decision_stage,actor_id,decision,reason,previous_state,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, value.ID, value.TenantID, value.ProjectID, value.SubjectType, value.SubjectID, value.SubjectHash, defaultDecisionStage(value.DecisionStage), value.ActorID, value.Decision, value.Reason, value.PreviousState, value.ResultingState, value.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) Approvals(ctx context.Context, tenantID, subjectID string) ([]domain.ApprovalDecision, error) {
	result := []domain.ApprovalDecision{}
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
			var value domain.ApprovalDecision
			if err := rows.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.SubjectType, &value.SubjectID, &value.SubjectHash, &value.DecisionStage, &value.ActorID, &value.Decision, &value.Reason, &value.PreviousState, &value.ResultingState, &value.CreatedAt); err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func defaultDecisionStage(value string) string {
	if strings.TrimSpace(value) == "" {
		return "internal"
	}
	return value
}

func (s *Store) AppendAudit(ctx context.Context, value domain.AuditEvent) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,tenant_id,project_id,actor_type,actor_id,action,subject_type,subject_id,summary,request_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, value.ID, value.TenantID, nullable(value.ProjectID), value.ActorType, value.ActorID, value.Action, value.SubjectType, value.SubjectID, jsonValue(value.Summary), value.RequestID, value.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) AuditEvents(ctx context.Context, tenantID, projectID string, limit int) ([]domain.AuditEvent, error) {
	result := []domain.AuditEvent{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := `SELECT id,tenant_id,COALESCE(project_id::text,''),actor_type,actor_id,action,subject_type,subject_id,summary,request_id,created_at FROM audit_events WHERE tenant_id=$1`
		args := []any{tenantID}
		if projectID != "" {
			query += ` AND project_id=$2`
			args = append(args, projectID)
		}
		query += ` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(len(args)+1)
		args = append(args, limit)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value domain.AuditEvent
			var summary []byte
			if err := rows.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ActorType, &value.ActorID, &value.Action, &value.SubjectType, &value.SubjectID, &summary, &value.RequestID, &value.CreatedAt); err != nil {
				return err
			}
			value.Summary, err = decodeJSON[map[string]any](summary)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}
