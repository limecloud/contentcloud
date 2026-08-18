package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func (s *Store) CreateUserDeviceFlow(ctx context.Context, v workspacedomain.UserDeviceFlow) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO user_device_flows(id,device_code_hash,user_code,user_id,tenant_id,state,expires_at,approved_at,consumed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, v.ID, v.DeviceCodeHash, v.UserCode, nullable(v.UserID), nullable(v.TenantID), v.State, v.ExpiresAt, v.ApprovedAt, v.ConsumedAt)
	return dbError(err)
}

func scanUserDeviceFlow(row pgx.Row) (workspacedomain.UserDeviceFlow, error) {
	var v workspacedomain.UserDeviceFlow
	err := row.Scan(&v.ID, &v.DeviceCodeHash, &v.UserCode, &v.UserID, &v.TenantID, &v.State, &v.ExpiresAt, &v.ApprovedAt, &v.ConsumedAt)
	return v, err
}

func (s *Store) UserDeviceFlowByCodeHash(ctx context.Context, hash string) (workspacedomain.UserDeviceFlow, error) {
	v, err := scanUserDeviceFlow(s.pool.QueryRow(ctx, `SELECT id,device_code_hash,user_code,COALESCE(user_id::text,''),COALESCE(tenant_id::text,''),state,expires_at,approved_at,consumed_at FROM user_device_flows WHERE device_code_hash=$1`, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fault.NotFound("登录授权")
	}
	return v, dbError(err)
}

func (s *Store) UserDeviceFlowByUserCode(ctx context.Context, code string) (workspacedomain.UserDeviceFlow, error) {
	v, err := scanUserDeviceFlow(s.pool.QueryRow(ctx, `SELECT id,device_code_hash,user_code,COALESCE(user_id::text,''),COALESCE(tenant_id::text,''),state,expires_at,approved_at,consumed_at FROM user_device_flows WHERE upper(user_code)=upper($1)`, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fault.NotFound("登录授权")
	}
	return v, dbError(err)
}

func (s *Store) SaveUserDeviceFlow(ctx context.Context, v workspacedomain.UserDeviceFlow) error {
	result, err := s.pool.Exec(ctx, `UPDATE user_device_flows SET user_id=$2,tenant_id=$3,state=$4,expires_at=$5,approved_at=$6,consumed_at=$7 WHERE id=$1`, v.ID, nullable(v.UserID), nullable(v.TenantID), v.State, v.ExpiresAt, v.ApprovedAt, v.ConsumedAt)
	if err != nil {
		return dbError(err)
	}
	if result.RowsAffected() == 0 {
		return fault.NotFound("登录授权")
	}
	return nil
}

func (s *Store) CreateCLIToken(ctx context.Context, v workspacedomain.CLIToken) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO cli_tokens(id,user_id,tenant_id,token_hash,expires_at,revoked_at) VALUES($1,$2,$3,$4,$5,$6)`, v.ID, v.UserID, v.TenantID, v.TokenHash, v.ExpiresAt, v.RevokedAt)
		return dbError(err)
	})
}

func scanCLIToken(row pgx.Row) (workspacedomain.CLIToken, error) {
	var v workspacedomain.CLIToken
	err := row.Scan(&v.ID, &v.UserID, &v.TenantID, &v.TokenHash, &v.ExpiresAt, &v.RevokedAt)
	return v, err
}

func (s *Store) CLITokenByHash(ctx context.Context, hash string) (workspacedomain.CLIToken, error) {
	var tenantID, tokenID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,token_id FROM contentcloud_lookup_cli_token($1)`, hash).Scan(&tenantID, &tokenID); err != nil {
		return workspacedomain.CLIToken{}, fault.NotFound("CLI 凭据")
	}
	var result workspacedomain.CLIToken
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanCLIToken(tx.QueryRow(ctx, `SELECT id,user_id,tenant_id,token_hash,expires_at,revoked_at FROM cli_tokens WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL AND expires_at>now()`, tenantID, tokenID))
		result = v
		return dbError(err)
	})
	return result, err
}

func (s *Store) RevokeCLIToken(ctx context.Context, hash string, now time.Time) error {
	v, err := s.CLITokenByHash(ctx, hash)
	if err != nil {
		return err
	}
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE cli_tokens SET revoked_at=$3 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, now)
		return err
	})
}
