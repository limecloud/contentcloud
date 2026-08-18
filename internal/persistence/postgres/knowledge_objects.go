package postgres

import (
	"context"
	"errors"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func (s *Store) CreateKnowledgeObject(ctx context.Context, value sourcedomain.KnowledgeObject) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Digest == "" {
		digest, err := value.ContentDigest()
		if err != nil {
			return err
		}
		value.Digest = digest
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		return insertKnowledgeObject(ctx, tx, value)
	})
}

func insertKnowledgeObject(ctx context.Context, tx pgx.Tx, value sourcedomain.KnowledgeObject) error {
	_, err := tx.Exec(ctx, `INSERT INTO knowledge_objects(tenant_id,project_id,id,version,object_type,layer,status,title,statement,payload,dimensions,allowed_channels,evidence_refs,relation_refs,rights_refs,conflict_refs,decision_ref,next_action,impact,valid_from,valid_until,expires_at,digest,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)`, value.TenantID, value.ProjectID, value.ID, value.Version, value.ObjectType, value.Layer, value.Status, value.Title, value.Statement, jsonValue(value.Payload), jsonArrayValue(value.Dimensions), jsonArrayValue(value.AllowedChannels), jsonArrayValue(value.EvidenceRefs), jsonArrayValue(value.RelationRefs), jsonArrayValue(value.RightsRefs), jsonArrayValue(value.ConflictRefs), value.DecisionRef, value.NextAction, value.Impact, value.ValidFrom, value.ValidUntil, value.ExpiresAt, value.Digest, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
	return dbError(err)
}

func (s *Store) CreateKnowledgeObjectDecision(ctx context.Context, object sourcedomain.KnowledgeObject, decision sourcedomain.KnowledgeDecision) error {
	if err := object.Validate(); err != nil {
		return err
	}
	if err := decision.Validate(); err != nil {
		return err
	}
	if decision.TenantID != object.TenantID || decision.ProjectID != object.ProjectID || decision.ObjectID != object.ID || decision.ResultVersion != object.Version {
		return fault.Invalid("KNOWLEDGE_DECISION_INVALID", "知识决策与结果对象版本不一致")
	}
	return s.withTenant(ctx, object.TenantID, func(tx pgx.Tx) error {
		var previousDigest string
		if err := tx.QueryRow(ctx, `SELECT digest FROM knowledge_objects WHERE tenant_id=$1 AND id=$2 AND version=$3`, object.TenantID, object.ID, decision.PreviousVersion).Scan(&previousDigest); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fault.NotFound("知识决策原对象")
			}
			return err
		}
		if previousDigest != decision.SubjectDigest {
			return fault.Conflict("KNOWLEDGE_DECISION_SUBJECT_CHANGED", "知识对象版本或摘要（digest）已变化")
		}
		if err := insertKnowledgeObject(ctx, tx, object); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO knowledge_decisions(tenant_id,id,project_id,object_id,previous_version,result_version,subject_digest,decision,reason,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, decision.TenantID, decision.ID, decision.ProjectID, decision.ObjectID, decision.PreviousVersion, decision.ResultVersion, decision.SubjectDigest, decision.Decision, decision.Reason, decision.ActorID, decision.CreatedAt)
		return dbError(err)
	})
}

const knowledgeObjectSelect = `SELECT id,tenant_id,project_id,object_type,layer,version,status,title,statement,payload,dimensions,allowed_channels,evidence_refs,relation_refs,rights_refs,conflict_refs,decision_ref,next_action,impact,valid_from,valid_until,expires_at,digest,created_by,created_at,updated_at FROM knowledge_objects`

func scanKnowledgeObject(row pgx.Row) (sourcedomain.KnowledgeObject, error) {
	var value sourcedomain.KnowledgeObject
	var payload, dimensions, channels, evidence, relations, rights, conflicts []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ObjectType, &value.Layer, &value.Version, &value.Status, &value.Title, &value.Statement, &payload, &dimensions, &channels, &evidence, &relations, &rights, &conflicts, &value.DecisionRef, &value.NextAction, &value.Impact, &value.ValidFrom, &value.ValidUntil, &value.ExpiresAt, &value.Digest, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.Payload, err = decodeJSON[map[string]any](payload)
	}
	if err == nil {
		value.Dimensions, err = decodeJSON[[]string](dimensions)
	}
	if err == nil {
		value.AllowedChannels, err = decodeJSON[[]string](channels)
	}
	if err == nil {
		value.EvidenceRefs, err = decodeJSON[[]string](evidence)
	}
	if err == nil {
		value.RelationRefs, err = decodeJSON[[]string](relations)
	}
	if err == nil {
		value.RightsRefs, err = decodeJSON[[]string](rights)
	}
	if err == nil {
		value.ConflictRefs, err = decodeJSON[[]string](conflicts)
	}
	return value, err
}

func (s *Store) KnowledgeObjects(ctx context.Context, tenantID, projectID string) ([]sourcedomain.KnowledgeObject, error) {
	result := []sourcedomain.KnowledgeObject{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, knowledgeObjectSelect+` WHERE tenant_id=$1 AND project_id=$2 ORDER BY id,version`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanKnowledgeObject(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) KnowledgeObject(ctx context.Context, tenantID, objectID string, version int) (sourcedomain.KnowledgeObject, error) {
	var result sourcedomain.KnowledgeObject
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := knowledgeObjectSelect + ` WHERE tenant_id=$1 AND id=$2`
		args := []any{tenantID, objectID}
		if version > 0 {
			query += ` AND version=$3`
			args = append(args, version)
		} else {
			query += ` ORDER BY version DESC LIMIT 1`
		}
		value, err := scanKnowledgeObject(tx.QueryRow(ctx, query, args...))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("知识对象")
		}
		result = value
		return err
	})
	return result, err
}

func (s *Store) CreateKnowledgePack(ctx context.Context, value sourcedomain.KnowledgePack) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Digest == "" {
		digest, err := value.ContentDigest()
		if err != nil {
			return err
		}
		value.Digest = digest
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO knowledge_packs(tenant_id,id,project_id,name,purpose,version,status,object_refs,query_policy,digest,created_by,published_by,created_at,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, value.TenantID, value.ID, value.ProjectID, value.Name, value.Purpose, value.Version, value.Status, jsonArrayValue(value.ObjectRefs), jsonValue(value.QueryPolicy), value.Digest, value.CreatedBy, value.PublishedBy, value.CreatedAt, value.PublishedAt)
		return dbError(err)
	})
}

const knowledgePackSelect = `SELECT id,tenant_id,project_id,name,purpose,version,status,object_refs,query_policy,digest,created_by,published_by,created_at,published_at FROM knowledge_packs`

func scanKnowledgePack(row pgx.Row) (sourcedomain.KnowledgePack, error) {
	var value sourcedomain.KnowledgePack
	var refs, policy []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.Name, &value.Purpose, &value.Version, &value.Status, &refs, &policy, &value.Digest, &value.CreatedBy, &value.PublishedBy, &value.CreatedAt, &value.PublishedAt)
	if err == nil {
		value.ObjectRefs, err = decodeJSON[[]sourcedomain.KnowledgePackObjectRef](refs)
	}
	if err == nil {
		value.QueryPolicy, err = decodeJSON[sourcedomain.KnowledgeQueryPolicy](policy)
	}
	return value, err
}

func (s *Store) KnowledgePacks(ctx context.Context, tenantID, projectID string) ([]sourcedomain.KnowledgePack, error) {
	result := []sourcedomain.KnowledgePack{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, knowledgePackSelect+` WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanKnowledgePack(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) KnowledgePack(ctx context.Context, tenantID, id string) (sourcedomain.KnowledgePack, error) {
	var result sourcedomain.KnowledgePack
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanKnowledgePack(tx.QueryRow(ctx, knowledgePackSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("知识包")
		}
		result = value
		return err
	})
	return result, err
}

func (s *Store) SaveKnowledgePack(ctx context.Context, value sourcedomain.KnowledgePack) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE knowledge_packs SET name=$3,purpose=$4,version=$5,status=$6,object_refs=$7,query_policy=$8,digest=$9,published_by=$10,published_at=$11 WHERE tenant_id=$1 AND id=$2 AND status='draft'`, value.TenantID, value.ID, value.Name, value.Purpose, value.Version, value.Status, jsonArrayValue(value.ObjectRefs), jsonValue(value.QueryPolicy), value.Digest, value.PublishedBy, value.PublishedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.Conflict("KNOWLEDGE_PACK_IMMUTABLE", "知识包不存在或已不可修改")
		}
		return nil
	})
}

func (s *Store) CreateKnowledgeSnapshot(ctx context.Context, value sourcedomain.KnowledgeSnapshot) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO knowledge_snapshots(tenant_id,id,project_id,pack_id,pack_version,pack_digest,objects,digest,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, value.TenantID, value.ID, value.ProjectID, value.PackID, value.PackVersion, value.PackDigest, jsonArrayValue(value.Objects), value.Digest, value.CreatedBy, value.CreatedAt)
		return dbError(err)
	})
}

const knowledgeSnapshotSelect = `SELECT id,tenant_id,project_id,pack_id,pack_version,pack_digest,objects,digest,created_by,created_at FROM knowledge_snapshots`

func scanKnowledgeSnapshot(row pgx.Row) (sourcedomain.KnowledgeSnapshot, error) {
	var value sourcedomain.KnowledgeSnapshot
	var objects []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.PackID, &value.PackVersion, &value.PackDigest, &objects, &value.Digest, &value.CreatedBy, &value.CreatedAt)
	if err == nil {
		value.Objects, err = decodeJSON[[]sourcedomain.KnowledgeObject](objects)
	}
	return value, err
}

func (s *Store) KnowledgeSnapshots(ctx context.Context, tenantID, projectID, packID string) ([]sourcedomain.KnowledgeSnapshot, error) {
	result := []sourcedomain.KnowledgeSnapshot{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := knowledgeSnapshotSelect + ` WHERE tenant_id=$1 AND project_id=$2`
		args := []any{tenantID, projectID}
		if packID != "" {
			query += ` AND pack_id=$3`
			args = append(args, packID)
		}
		query += ` ORDER BY created_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanKnowledgeSnapshot(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) KnowledgeSnapshot(ctx context.Context, tenantID, id string) (sourcedomain.KnowledgeSnapshot, error) {
	var result sourcedomain.KnowledgeSnapshot
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanKnowledgeSnapshot(tx.QueryRow(ctx, knowledgeSnapshotSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("知识快照")
		}
		result = value
		return err
	})
	return result, err
}

func (s *Store) KnowledgeDecisions(ctx context.Context, tenantID, objectID string) ([]sourcedomain.KnowledgeDecision, error) {
	result := []sourcedomain.KnowledgeDecision{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,tenant_id,project_id,object_id,previous_version,result_version,subject_digest,decision,reason,actor_id,created_at FROM knowledge_decisions WHERE tenant_id=$1 AND object_id=$2 ORDER BY created_at DESC`, tenantID, objectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value sourcedomain.KnowledgeDecision
			if err := rows.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ObjectID, &value.PreviousVersion, &value.ResultVersion, &value.SubjectDigest, &value.Decision, &value.Reason, &value.ActorID, &value.CreatedAt); err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}
