package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateKnowledgeConflict(ctx context.Context, conflict domain.KnowledgeConflict, request domain.DecisionRequest) error {
	return s.withTenant(ctx, conflict.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO knowledge_conflicts(id,tenant_id,project_id,subject,predicate,knowledge_item_ids,reason,status,resolved_by,resolved_at,resolution,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, conflict.ID, conflict.TenantID, conflict.ProjectID, conflict.Subject, conflict.Predicate, jsonArrayValue(conflict.KnowledgeItemIDs), conflict.Reason, conflict.Status, nullable(conflict.ResolvedBy), conflict.ResolvedAt, conflict.Resolution, conflict.CreatedAt, conflict.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO decision_requests(id,tenant_id,project_id,conflict_id,question,knowledge_item_ids,status,requested_by,resolved_by,resolved_at,selected_knowledge_id,notes,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, request.ID, request.TenantID, request.ProjectID, request.ConflictID, request.Question, jsonArrayValue(request.KnowledgeItemIDs), request.Status, request.RequestedBy, nullable(request.ResolvedBy), request.ResolvedAt, nullable(request.SelectedKnowledgeID), request.Notes, request.CreatedAt)
		return dbError(err)
	})
}

func scanKnowledgeConflict(row pgx.Row) (domain.KnowledgeConflict, error) {
	var value domain.KnowledgeConflict
	var itemIDs []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.Subject, &value.Predicate, &itemIDs, &value.Reason, &value.Status, &value.ResolvedBy, &value.ResolvedAt, &value.Resolution, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.KnowledgeItemIDs, err = decodeJSON[[]string](itemIDs)
	}
	return value, err
}

const knowledgeConflictSelect = `SELECT id,tenant_id,project_id,subject,predicate,knowledge_item_ids,reason,status,COALESCE(resolved_by::text,''),resolved_at,resolution,created_at,updated_at FROM knowledge_conflicts`

func (s *Store) KnowledgeConflicts(ctx context.Context, tenantID, projectID string) ([]domain.KnowledgeConflict, error) {
	items := []domain.KnowledgeConflict{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, knowledgeConflictSelect+` WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanKnowledgeConflict(rows)
			if err != nil {
				return err
			}
			items = append(items, value)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Store) KnowledgeConflict(ctx context.Context, tenantID, id string) (domain.KnowledgeConflict, error) {
	var result domain.KnowledgeConflict
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanKnowledgeConflict(tx.QueryRow(ctx, knowledgeConflictSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("知识冲突")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveKnowledgeConflict(ctx context.Context, value domain.KnowledgeConflict) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE knowledge_conflicts SET knowledge_item_ids=$3,reason=$4,status=$5,resolved_by=$6,resolved_at=$7,resolution=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, jsonArrayValue(value.KnowledgeItemIDs), value.Reason, value.Status, nullable(value.ResolvedBy), value.ResolvedAt, value.Resolution, value.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("知识冲突")
		}
		return nil
	})
}

func scanDecisionRequest(row pgx.Row) (domain.DecisionRequest, error) {
	var value domain.DecisionRequest
	var itemIDs []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ConflictID, &value.Question, &itemIDs, &value.Status, &value.RequestedBy, &value.ResolvedBy, &value.ResolvedAt, &value.SelectedKnowledgeID, &value.Notes, &value.CreatedAt)
	if err == nil {
		value.KnowledgeItemIDs, err = decodeJSON[[]string](itemIDs)
	}
	return value, err
}

const decisionRequestSelect = `SELECT id,tenant_id,project_id,conflict_id,question,knowledge_item_ids,status,requested_by,COALESCE(resolved_by::text,''),resolved_at,COALESCE(selected_knowledge_id::text,''),notes,created_at FROM decision_requests`

func (s *Store) DecisionRequests(ctx context.Context, tenantID, projectID string) ([]domain.DecisionRequest, error) {
	items := []domain.DecisionRequest{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, decisionRequestSelect+` WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanDecisionRequest(rows)
			if err != nil {
				return err
			}
			items = append(items, value)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Store) DecisionRequest(ctx context.Context, tenantID, id string) (domain.DecisionRequest, error) {
	var result domain.DecisionRequest
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanDecisionRequest(tx.QueryRow(ctx, decisionRequestSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("决策请求")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveDecisionRequest(ctx context.Context, value domain.DecisionRequest) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE decision_requests SET status=$3,resolved_by=$4,resolved_at=$5,selected_knowledge_id=$6,notes=$7 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.Status, nullable(value.ResolvedBy), value.ResolvedAt, nullable(value.SelectedKnowledgeID), value.Notes)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("决策请求")
		}
		return nil
	})
}
