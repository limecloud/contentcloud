package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"

	// commitFaultInjector is intentionally opt-in and primarily used by
	// integration tests to model a process crash immediately after COMMIT.
	// Production callers never install one.
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
)

type Store struct {
	pool *pgxpool.Pool

	commitFaultInjector func(string) error
}

// ErrPostCommitFault marks the deliberately ambiguous result where the
// database commit succeeded but the caller lost its response. Callers must
// retry through an idempotency key instead of assuming the command rolled
// back.
var ErrPostCommitFault = errors.New("postgres transaction committed before response failure")

type Option func(*Store)

// WithCommitFaultInjector installs a test-only command transaction hook. The
// hook receives "<scope>:before_commit" or "<scope>:after_commit". Returning
// an error from an after_commit phase simulates a lost response after
// PostgreSQL accepted the command, so callers must recover through their
// idempotency contract. Ordinary read transactions never invoke this hook.
func WithCommitFaultInjector(injector func(string) error) Option {
	return func(store *Store) {
		store.commitFaultInjector = injector
	}
}

func New(ctx context.Context, databaseURL string, options ...Option) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	store := &Store{pool: pool}
	for _, option := range options {
		if option != nil {
			option(store)
		}
	}
	return store, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) withTenant(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	if tenantID == "" {
		return fault.Invalid("TENANT_CONTEXT_REQUIRED", "缺少租户上下文")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE contentcloud_runtime`); err != nil {
		return fmt.Errorf("启用 RLS 运行角色失败：%w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	scope, _ := ctx.Value(commitFaultScopeKey{}).(string)
	if scope != "" && s.commitFaultInjector != nil {
		if err := s.commitFaultInjector(scope + ":before_commit"); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if scope != "" && s.commitFaultInjector != nil {
		if err := s.commitFaultInjector(scope + ":after_commit"); err != nil {
			return fmt.Errorf("%w: %v", ErrPostCommitFault, err)
		}
	}
	return nil
}

type commitFaultScopeKey struct{}

func (s *Store) withTenantCommand(ctx context.Context, tenantID, scope string, fn func(pgx.Tx) error) error {
	return s.withTenant(context.WithValue(ctx, commitFaultScopeKey{}, scope), tenantID, fn)
}

func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fault.NotFound("资源")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fault.Conflict("RESOURCE_CONFLICT", "资源已存在或幂等键冲突")
		case "23503", "23514", "22P02":
			return fault.Invalid("DATABASE_CONSTRAINT", "请求不满足数据约束")
		}
	}
	return err
}

func jsonValue(value any) []byte {
	body, _ := json.Marshal(value)
	return body
}

func jsonArrayValue[T any](value []T) []byte {
	if value == nil {
		value = []T{}
	}
	return jsonValue(value)
}

func decodeJSON[T any](body []byte) (T, error) {
	var value T
	err := json.Unmarshal(body, &value)
	return value, err
}

func (s *Store) CreateUser(ctx context.Context, v identitydomain.User) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,verified_at,created_at) VALUES($1,$2,$3,$4,$5,$6)`, v.ID, strings.ToLower(v.Email), v.DisplayName, v.PasswordHash, v.VerifiedAt, v.CreatedAt)
	return dbError(err)
}

func (s *Store) UserByEmail(ctx context.Context, email string) (identitydomain.User, error) {
	var v identitydomain.User
	err := s.pool.QueryRow(ctx, `SELECT id,email,display_name,password_hash,verified_at,created_at FROM users WHERE email=$1`, strings.ToLower(email)).Scan(&v.ID, &v.Email, &v.DisplayName, &v.PasswordHash, &v.VerifiedAt, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fault.NotFound("用户")
	}
	return v, dbError(err)
}

func (s *Store) UserByID(ctx context.Context, id string) (identitydomain.User, error) {
	var v identitydomain.User
	err := s.pool.QueryRow(ctx, `SELECT id,email,display_name,password_hash,verified_at,created_at FROM users WHERE id=$1`, id).Scan(&v.ID, &v.Email, &v.DisplayName, &v.PasswordHash, &v.VerifiedAt, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fault.NotFound("用户")
	}
	return v, dbError(err)
}

func (s *Store) SaveUser(ctx context.Context, v identitydomain.User) error {
	result, err := s.pool.Exec(ctx, `UPDATE users SET email=$2,display_name=$3,password_hash=$4,verified_at=$5 WHERE id=$1`, v.ID, strings.ToLower(v.Email), v.DisplayName, v.PasswordHash, v.VerifiedAt)
	if err != nil {
		return dbError(err)
	}
	if result.RowsAffected() == 0 {
		return fault.NotFound("用户")
	}
	return nil
}

func (s *Store) SaveSession(ctx context.Context, v identitydomain.Session) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions(id,user_id,tenant_id,expires_at,revoked_at) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (id) DO UPDATE SET tenant_id=EXCLUDED.tenant_id,expires_at=EXCLUDED.expires_at,revoked_at=EXCLUDED.revoked_at`, v.ID, v.UserID, v.TenantID, v.ExpiresAt, v.RevokedAt)
	return dbError(err)
}

func (s *Store) SessionByID(ctx context.Context, id string) (identitydomain.Session, error) {
	var v identitydomain.Session
	err := s.pool.QueryRow(ctx, `SELECT id,user_id,tenant_id,expires_at,revoked_at FROM sessions WHERE id=$1 AND expires_at>now() AND revoked_at IS NULL`, id).Scan(&v.ID, &v.UserID, &v.TenantID, &v.ExpiresAt, &v.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fault.NotFound("会话")
	}
	return v, dbError(err)
}

func (s *Store) RevokeSession(ctx context.Context, id string, now time.Time) error {
	result, err := s.pool.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE id=$1 AND revoked_at IS NULL`, id, now)
	if err == nil && result.RowsAffected() == 0 {
		return fault.NotFound("会话")
	}
	return dbError(err)
}

func (s *Store) RevokeSessionsForUserTenant(ctx context.Context, userID, tenantID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET revoked_at=$3 WHERE user_id=$1 AND tenant_id=$2 AND revoked_at IS NULL`, userID, tenantID, now)
	return dbError(err)
}

func (s *Store) CreateTenant(ctx context.Context, tenant identitydomain.Tenant, membership identitydomain.Membership) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,slug,name,status,created_at) VALUES($1,$2,$3,$4,$5)`, tenant.ID, tenant.Slug, tenant.Name, tenant.Status, tenant.CreatedAt); err != nil {
		return dbError(err)
	}
	if membership.Status == "" {
		membership.Status = "active"
	}
	if membership.CreatedAt.IsZero() {
		membership.CreatedAt = tenant.CreatedAt
	}
	if _, err := tx.Exec(ctx, `INSERT INTO memberships(tenant_id,user_id,role,status,created_at,revoked_at) VALUES($1,$2,$3,$4,$5,$6)`, membership.TenantID, membership.UserID, membership.Role, membership.Status, membership.CreatedAt, membership.RevokedAt); err != nil {
		return dbError(err)
	}
	return tx.Commit(ctx)
}

func (s *Store) TenantsForUser(ctx context.Context, userID string) ([]identitydomain.Tenant, error) {
	rows, err := s.pool.Query(ctx, `SELECT t.id,t.slug,t.name,t.status,t.created_at FROM tenants t JOIN memberships m ON m.tenant_id=t.id WHERE m.user_id=$1 AND m.status='active' AND m.revoked_at IS NULL AND t.status='active' ORDER BY t.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []identitydomain.Tenant{}
	for rows.Next() {
		var v identitydomain.Tenant
		if err := rows.Scan(&v.ID, &v.Slug, &v.Name, &v.Status, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Membership(ctx context.Context, tenantID, userID string) (identitydomain.Membership, error) {
	var v identitydomain.Membership
	err := s.pool.QueryRow(ctx, `SELECT tenant_id,user_id,role,status,created_at,revoked_at FROM memberships WHERE tenant_id=$1 AND user_id=$2 AND status='active' AND revoked_at IS NULL`, tenantID, userID).Scan(&v.TenantID, &v.UserID, &v.Role, &v.Status, &v.CreatedAt, &v.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fault.NotFound("成员关系")
	}
	return v, dbError(err)
}

func (s *Store) Memberships(ctx context.Context, tenantID string) ([]identitydomain.Membership, error) {
	rows, err := s.pool.Query(ctx, `SELECT tenant_id,user_id,role,status,created_at,revoked_at FROM memberships WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []identitydomain.Membership{}
	for rows.Next() {
		var v identitydomain.Membership
		if err := rows.Scan(&v.TenantID, &v.UserID, &v.Role, &v.Status, &v.CreatedAt, &v.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) PlatformTenants(ctx context.Context) ([]identitydomain.PlatformTenant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id,t.slug,t.name,t.status,t.created_at,
			(SELECT count(*) FROM memberships m WHERE m.tenant_id=t.id AND m.status='active' AND m.revoked_at IS NULL),
			(SELECT count(*) FROM brand_projects p WHERE p.tenant_id=t.id),
			(SELECT count(*) FROM devices d WHERE d.tenant_id=t.id AND d.revoked_at IS NULL AND d.last_seen_at>now()-interval '2 minutes'),
			(SELECT count(*) FROM runtime_job_runs r
			 WHERE r.tenant_id=t.id AND r.state IN ('created','admitted','running','waiting_human')),
			(SELECT max(p.updated_at) FROM brand_projects p WHERE p.tenant_id=t.id),
			COALESCE((SELECT array_agg(c.content_type ORDER BY c.content_type) FROM tenant_content_capabilities c WHERE c.tenant_id=t.id AND c.enabled), '{}'::text[])
		FROM tenants t ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []identitydomain.PlatformTenant{}
	for rows.Next() {
		var value identitydomain.PlatformTenant
		var optionalContentTypes []string
		if err := rows.Scan(&value.ID, &value.Slug, &value.Name, &value.Status, &value.CreatedAt, &value.MemberCount, &value.ProjectCount, &value.DeviceCount, &value.ActiveRunCount, &value.LastActivityAt, &optionalContentTypes); err != nil {
			return nil, err
		}
		capabilities := make([]identitydomain.TenantContentCapability, 0, len(optionalContentTypes))
		for _, contentType := range optionalContentTypes {
			capabilities = append(capabilities, identitydomain.TenantContentCapability{ContentType: contentType, Enabled: true})
		}
		value.ContentTypes = identitydomain.EnabledTenantContentTypes(capabilities)
		out = append(out, value)
	}
	return out, rows.Err()
}

func (s *Store) TenantContentCapabilities(ctx context.Context, tenantID string) ([]identitydomain.TenantContentCapability, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1)`, tenantID).Scan(&exists); err != nil {
		return nil, dbError(err)
	}
	if !exists {
		return nil, fault.NotFound("租户")
	}
	rows, err := s.pool.Query(ctx, `SELECT tenant_id,content_type,enabled,updated_by,updated_at FROM tenant_content_capabilities WHERE tenant_id=$1 ORDER BY content_type`, tenantID)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	values := []identitydomain.TenantContentCapability{}
	for rows.Next() {
		var value identitydomain.TenantContentCapability
		if err := rows.Scan(&value.TenantID, &value.ContentType, &value.Enabled, &value.UpdatedBy, &value.UpdatedAt); err != nil {
			return nil, dbError(err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) SetTenantContentCapability(ctx context.Context, value identitydomain.TenantContentCapability) error {
	var updated bool
	err := s.pool.QueryRow(ctx, `
		WITH upserted AS (
			INSERT INTO tenant_content_capabilities(tenant_id,content_type,enabled,updated_by,updated_at)
			SELECT id,$2,$3,$4,$5 FROM tenants WHERE id=$1
			ON CONFLICT(tenant_id,content_type) DO UPDATE
			SET enabled=EXCLUDED.enabled,updated_by=EXCLUDED.updated_by,updated_at=EXCLUDED.updated_at
			RETURNING tenant_id
		)
		SELECT EXISTS(SELECT 1 FROM upserted)`, value.TenantID, value.ContentType, value.Enabled, value.UpdatedBy, value.UpdatedAt).Scan(&updated)
	if err != nil {
		return dbError(err)
	}
	if !updated {
		return fault.NotFound("租户")
	}
	return nil
}

func (s *Store) PlatformUsers(ctx context.Context) ([]identitydomain.PlatformUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,email,display_name,verified_at,created_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	out := []identitydomain.PlatformUser{}
	index := map[string]int{}
	for rows.Next() {
		var value identitydomain.PlatformUser
		if err := rows.Scan(&value.ID, &value.Email, &value.DisplayName, &value.VerifiedAt, &value.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		value.Memberships = []identitydomain.PlatformUserMembership{}
		index[value.ID] = len(out)
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	membershipRows, err := s.pool.Query(ctx, `SELECT m.user_id,m.tenant_id,t.name,m.role,m.status FROM memberships m JOIN tenants t ON t.id=m.tenant_id ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer membershipRows.Close()
	for membershipRows.Next() {
		var userID string
		var membership identitydomain.PlatformUserMembership
		if err := membershipRows.Scan(&userID, &membership.TenantID, &membership.TenantName, &membership.Role, &membership.Status); err != nil {
			return nil, err
		}
		if position, ok := index[userID]; ok {
			out[position].Memberships = append(out[position].Memberships, membership)
		}
	}
	return out, membershipRows.Err()
}

func (s *Store) SetTenantStatus(ctx context.Context, tenantID, status string, now time.Time) (identitydomain.Tenant, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return identitydomain.Tenant{}, err
	}
	defer tx.Rollback(ctx)
	var tenant identitydomain.Tenant
	err = tx.QueryRow(ctx, `UPDATE tenants SET status=$2 WHERE id=$1 RETURNING id,slug,name,status,created_at`, tenantID, status).Scan(&tenant.ID, &tenant.Slug, &tenant.Name, &tenant.Status, &tenant.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenant, fault.NotFound("租户")
	}
	if err != nil {
		return tenant, dbError(err)
	}
	if status == "suspended" {
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE tenant_id=$1 AND revoked_at IS NULL`, tenantID, now); err != nil {
			return tenant, dbError(err)
		}
	}
	return tenant, tx.Commit(ctx)
}

func (s *Store) SaveMembership(ctx context.Context, v identitydomain.Membership) error {
	if v.Status == "" {
		v.Status = "active"
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO memberships(tenant_id,user_id,role,status,created_at,revoked_at) VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id,user_id) DO UPDATE SET role=EXCLUDED.role,status=EXCLUDED.status,revoked_at=EXCLUDED.revoked_at`, v.TenantID, v.UserID, v.Role, v.Status, v.CreatedAt, v.RevokedAt)
	return dbError(err)
}

func pendingMembershipInvite(ctx context.Context, tx pgx.Tx, tokenHash, email string, now time.Time) (identitydomain.MembershipInvite, error) {
	var tenantID, inviteID string
	if err := tx.QueryRow(ctx, `SELECT tenant_id,invite_id FROM contentcloud_lookup_membership_invite($1)`, tokenHash).Scan(&tenantID, &inviteID); err != nil {
		return identitydomain.MembershipInvite{}, fault.Conflict("INVITE_INVALID", "邀请无效、已撤销、邮箱不匹配或已过期")
	}
	invite, err := scanMembershipInvite(tx.QueryRow(ctx, membershipInviteSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, inviteID))
	if err != nil {
		return identitydomain.MembershipInvite{}, fault.Conflict("INVITE_INVALID", "邀请无效、已撤销、邮箱不匹配或已过期")
	}
	if err := invite.ValidateAcceptance(email, now); err != nil {
		return identitydomain.MembershipInvite{}, err
	}
	return invite, nil
}

func redeemMembershipInvite(ctx context.Context, tx pgx.Tx, invite identitydomain.MembershipInvite, userID string, now time.Time) (identitydomain.Membership, error) {
	membership := identitydomain.Membership{TenantID: invite.TenantID, UserID: userID, Role: invite.Role, Status: "active", CreatedAt: now}
	if _, err := tx.Exec(ctx, `INSERT INTO memberships(tenant_id,user_id,role,status,created_at,revoked_at) VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id,user_id) DO UPDATE SET role=EXCLUDED.role,status=EXCLUDED.status,revoked_at=EXCLUDED.revoked_at`, membership.TenantID, membership.UserID, membership.Role, membership.Status, membership.CreatedAt, membership.RevokedAt); err != nil {
		return identitydomain.Membership{}, dbError(err)
	}
	result, err := tx.Exec(ctx, `UPDATE membership_invites SET status='accepted',accepted_by=$2,accepted_at=$3 WHERE id=$1 AND status='pending'`, invite.ID, userID, now)
	if err != nil {
		return identitydomain.Membership{}, dbError(err)
	}
	if result.RowsAffected() != 1 {
		return identitydomain.Membership{}, fault.Conflict("INVITE_INVALID", "邀请无效、已撤销、邮箱不匹配或已过期")
	}
	return membership, nil
}

func (s *Store) AcceptMembershipInvite(ctx context.Context, tokenHash string, user identitydomain.User, now time.Time) (identitydomain.Membership, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return identitydomain.Membership{}, err
	}
	defer tx.Rollback(ctx)
	invite, err := pendingMembershipInvite(ctx, tx, tokenHash, user.Email, now)
	if err != nil {
		return identitydomain.Membership{}, err
	}
	membership, err := redeemMembershipInvite(ctx, tx, invite, user.ID, now)
	if err != nil {
		return identitydomain.Membership{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return identitydomain.Membership{}, err
	}
	return membership, nil
}

func (s *Store) RegisterWithInvite(ctx context.Context, user identitydomain.User, tokenHash string, session identitydomain.Session, now time.Time) (identitydomain.Session, identitydomain.Membership, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return identitydomain.Session{}, identitydomain.Membership{}, err
	}
	defer tx.Rollback(ctx)
	invite, err := pendingMembershipInvite(ctx, tx, tokenHash, user.Email, now)
	if err != nil {
		return identitydomain.Session{}, identitydomain.Membership{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users(id,email,display_name,password_hash,verified_at,created_at) VALUES($1,$2,$3,$4,$5,$6)`, user.ID, strings.ToLower(user.Email), user.DisplayName, user.PasswordHash, user.VerifiedAt, user.CreatedAt); err != nil {
		return identitydomain.Session{}, identitydomain.Membership{}, dbError(err)
	}
	membership, err := redeemMembershipInvite(ctx, tx, invite, user.ID, now)
	if err != nil {
		return identitydomain.Session{}, identitydomain.Membership{}, err
	}
	session.UserID, session.TenantID = user.ID, invite.TenantID
	if _, err := tx.Exec(ctx, `INSERT INTO sessions(id,user_id,tenant_id,expires_at,revoked_at) VALUES($1,$2,$3,$4,$5)`, session.ID, session.UserID, session.TenantID, session.ExpiresAt, session.RevokedAt); err != nil {
		return identitydomain.Session{}, identitydomain.Membership{}, dbError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return identitydomain.Session{}, identitydomain.Membership{}, err
	}
	return session, membership, nil
}

func (s *Store) CreateMembershipInvite(ctx context.Context, v identitydomain.MembershipInvite) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO membership_invites(id,tenant_id,email,role,invited_by,token_hash,status,expires_at,accepted_by,accepted_at,revoked_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, v.ID, v.TenantID, strings.ToLower(v.Email), v.Role, v.InvitedBy, v.TokenHash, v.Status, v.ExpiresAt, nullable(v.AcceptedBy), v.AcceptedAt, v.RevokedAt, v.CreatedAt)
		return dbError(err)
	})
}

func scanMembershipInvite(row pgx.Row) (identitydomain.MembershipInvite, error) {
	var v identitydomain.MembershipInvite
	err := row.Scan(&v.ID, &v.TenantID, &v.Email, &v.Role, &v.InvitedBy, &v.TokenHash, &v.Status, &v.ExpiresAt, &v.AcceptedBy, &v.AcceptedAt, &v.RevokedAt, &v.CreatedAt)
	return v, err
}

const membershipInviteSelect = `SELECT id,tenant_id,email,role,invited_by,token_hash,status,expires_at,COALESCE(accepted_by::text,''),accepted_at,revoked_at,created_at FROM membership_invites`

func (s *Store) MembershipInviteByTokenHash(ctx context.Context, hash string) (identitydomain.MembershipInvite, error) {
	var tenantID, inviteID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,invite_id FROM contentcloud_lookup_membership_invite($1)`, hash).Scan(&tenantID, &inviteID); err != nil {
		return identitydomain.MembershipInvite{}, fault.NotFound("成员邀请")
	}
	var result identitydomain.MembershipInvite
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanMembershipInvite(tx.QueryRow(ctx, membershipInviteSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, inviteID))
		result = v
		return dbError(err)
	})
	return result, err
}

func (s *Store) MembershipInvites(ctx context.Context, tenantID string) ([]identitydomain.MembershipInvite, error) {
	out := []identitydomain.MembershipInvite{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, membershipInviteSelect+` WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanMembershipInvite(rows)
			if err != nil {
				return err
			}
			v.TokenHash = ""
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) SaveMembershipInvite(ctx context.Context, v identitydomain.MembershipInvite) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE membership_invites SET role=$3,status=$4,expires_at=$5,accepted_by=$6,accepted_at=$7,revoked_at=$8 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.Role, v.Status, v.ExpiresAt, nullable(v.AcceptedBy), v.AcceptedAt, v.RevokedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.NotFound("成员邀请")
		}
		return nil
	})
}

const projectSelect = `SELECT p.id,p.tenant_id,p.slug,p.brand_name,p.product_name,p.content_type,p.channel,p.stage_objective,p.status,p.owner_name,p.reviewer_name,p.client_approver,p.row_version,p.created_at,p.updated_at,
  (SELECT count(*) FROM project_device_grants g JOIN devices d ON d.id=g.device_id WHERE g.project_id=p.id AND g.revoked_at IS NULL AND d.revoked_at IS NULL),
  (SELECT count(*) FROM knowledge_objects k WHERE k.project_id=p.id AND k.status IN ('verified','approved','valid','active')),
  (SELECT count(*) FROM knowledge_objects k WHERE k.project_id=p.id AND k.status IN ('candidate','needs_review','conflicted','blocked','open'))
  FROM brand_projects p`

func scanProject(row pgx.Row) (workspacedomain.Project, error) {
	var v workspacedomain.Project
	err := row.Scan(&v.ID, &v.TenantID, &v.Slug, &v.BrandName, &v.ProductName, &v.ContentType, &v.Channel, &v.StageObjective, &v.Status, &v.OwnerName, &v.ReviewerName, &v.ClientApprover, &v.RowVersion, &v.CreatedAt, &v.UpdatedAt, &v.ConnectedDevices, &v.KnowledgeReady, &v.OpenBlockers)
	return v, err
}

func (s *Store) CreateProject(ctx context.Context, v workspacedomain.Project) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO brand_projects(id,tenant_id,slug,brand_name,product_name,content_type,channel,stage_objective,status,owner_name,reviewer_name,client_approver,row_version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, v.ID, v.TenantID, v.Slug, v.BrandName, v.ProductName, v.ContentType, v.Channel, v.StageObjective, v.Status, v.OwnerName, v.ReviewerName, v.ClientApprover, v.RowVersion, v.CreatedAt, v.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) Projects(ctx context.Context, tenantID string) ([]workspacedomain.Project, error) {
	out := []workspacedomain.Project{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, projectSelect+` WHERE p.tenant_id=$1 ORDER BY p.updated_at DESC`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanProject(rows)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) Project(ctx context.Context, tenantID, id string) (workspacedomain.Project, error) {
	var result workspacedomain.Project
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanProject(tx.QueryRow(ctx, projectSelect+` WHERE p.tenant_id=$1 AND p.id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("项目")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveProject(ctx context.Context, v workspacedomain.Project) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE brand_projects SET slug=$3,brand_name=$4,product_name=$5,content_type=$6,channel=$7,stage_objective=$8,status=$9,owner_name=$10,reviewer_name=$11,client_approver=$12,row_version=$13,updated_at=$14 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.Slug, v.BrandName, v.ProductName, v.ContentType, v.Channel, v.StageObjective, v.Status, v.OwnerName, v.ReviewerName, v.ClientApprover, v.RowVersion, v.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.NotFound("项目")
		}
		return nil
	})
}

func (s *Store) UpdateProject(ctx context.Context, v workspacedomain.Project, expectedVersion int) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE brand_projects SET slug=$3,brand_name=$4,product_name=$5,content_type=$6,channel=$7,stage_objective=$8,status=$9,owner_name=$10,reviewer_name=$11,client_approver=$12,row_version=$13,updated_at=$14 WHERE tenant_id=$1 AND id=$2 AND row_version=$15`, v.TenantID, v.ID, v.Slug, v.BrandName, v.ProductName, v.ContentType, v.Channel, v.StageObjective, v.Status, v.OwnerName, v.ReviewerName, v.ClientApprover, v.RowVersion, v.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_projects WHERE tenant_id=$1 AND id=$2)`, v.TenantID, v.ID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return fault.NotFound("项目")
			}
			return fault.Conflict("ROW_VERSION_CONFLICT", "项目已被其他用户修改")
		}
		return nil
	})
}

func (s *Store) CreateProjectTemplate(ctx context.Context, v workspacedomain.ProjectTemplate) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO project_templates(id,tenant_id,name,channel,stage_objective,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, v.ID, v.TenantID, v.Name, v.Channel, v.StageObjective, v.CreatedBy, v.CreatedAt)
		return dbError(err)
	})
}

func scanProjectTemplate(row pgx.Row) (workspacedomain.ProjectTemplate, error) {
	var v workspacedomain.ProjectTemplate
	err := row.Scan(&v.ID, &v.TenantID, &v.Name, &v.Channel, &v.StageObjective, &v.CreatedBy, &v.CreatedAt)
	return v, err
}

const projectTemplateSelect = `SELECT id,tenant_id,name,channel,stage_objective,created_by,created_at FROM project_templates`

func (s *Store) ProjectTemplates(ctx context.Context, tenantID string) ([]workspacedomain.ProjectTemplate, error) {
	out := []workspacedomain.ProjectTemplate{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, projectTemplateSelect+` WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanProjectTemplate(rows)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) ProjectTemplate(ctx context.Context, tenantID, id string) (workspacedomain.ProjectTemplate, error) {
	var result workspacedomain.ProjectTemplate
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanProjectTemplate(tx.QueryRow(ctx, projectTemplateSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("项目模板")
		}
		return err
	})
	return result, err
}

func (s *Store) CreateConnectSession(ctx context.Context, v workspacedomain.ConnectSession) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO connect_sessions(id,tenant_id,project_id,inviter_user_id,state,expires_at,consumed_at,consumed_device_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, v.TenantID, v.ProjectID, v.InviterUserID, v.State, v.ExpiresAt, v.ConsumedAt, nullable(v.ConsumedDeviceID))
		return dbError(err)
	})
}

func scanConnect(row pgx.Row) (workspacedomain.ConnectSession, error) {
	var v workspacedomain.ConnectSession
	err := row.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.InviterUserID, &v.State, &v.ExpiresAt, &v.ConsumedAt, &v.ConsumedDeviceID)
	return v, err
}

func (s *Store) ConnectSessionByID(ctx context.Context, tenantID, id string) (workspacedomain.ConnectSession, error) {
	var result workspacedomain.ConnectSession
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanConnect(tx.QueryRow(ctx, `SELECT id,tenant_id,project_id,inviter_user_id,state,expires_at,consumed_at,COALESCE(consumed_device_id::text,'') FROM connect_sessions WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("连接会话")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveConnectSession(ctx context.Context, v workspacedomain.ConnectSession) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE connect_sessions SET state=$3,expires_at=$4,consumed_at=$5,consumed_device_id=$6 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.State, v.ExpiresAt, v.ConsumedAt, nullable(v.ConsumedDeviceID))
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.NotFound("连接会话")
		}
		return nil
	})
}

func (s *Store) SaveDevice(ctx context.Context, v workspacedomain.Device) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE devices SET display_name=$3,hostname=$4,platform=$5,arch=$6,daemon_version=$7,capability_manifests=$8,last_seen_at=$9,revoked_at=$10 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.DisplayName, v.Hostname, v.Platform, v.Arch, v.Version, jsonArrayValue(v.Capabilities), v.LastSeenAt, v.RevokedAt)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return fault.NotFound("设备")
		}
		_, err = tx.Exec(ctx, `UPDATE connect_sessions SET state='connected' WHERE tenant_id=$1 AND consumed_device_id=$2 AND state='verifying'`, v.TenantID, v.ID)
		return err
	})
}

func (s *Store) RotateDeviceCredential(ctx context.Context, tenantID, deviceID, tokenHash string, now time.Time) (workspacedomain.Device, error) {
	var result workspacedomain.Device
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `UPDATE devices SET token_hash=$3,credential_version=credential_version+1,credential_rotated_at=$4 WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL RETURNING id,tenant_id,owner_user_id,machine_id,display_name,hostname,platform,arch,daemon_version,token_hash,credential_version,credential_rotated_at,capability_manifests,'[]'::jsonb,last_seen_at,revoked_at`, tenantID, deviceID, tokenHash, now)
		value, err := scanDevice(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("设备")
		}
		result = value
		return err
	})
	result.TokenHash = ""
	return result, err
}

func scanDevice(row pgx.Row) (workspacedomain.Device, error) {
	var v workspacedomain.Device
	var capabilities, projectIDs []byte
	err := row.Scan(&v.ID, &v.TenantID, &v.OwnerUserID, &v.MachineID, &v.DisplayName, &v.Hostname, &v.Platform, &v.Arch, &v.Version, &v.TokenHash, &v.CredentialVersion, &v.CredentialRotatedAt, &capabilities, &projectIDs, &v.LastSeenAt, &v.RevokedAt)
	if err == nil {
		v.Capabilities, err = decodeJSON[[]catalogdomain.Capability](capabilities)
	}
	if err == nil {
		v.ProjectIDs, err = decodeJSON[[]string](projectIDs)
	}
	return v, err
}

const deviceSelect = `SELECT d.id,d.tenant_id,d.owner_user_id,d.machine_id,d.display_name,d.hostname,d.platform,d.arch,d.daemon_version,d.token_hash,d.credential_version,d.credential_rotated_at,d.capability_manifests,
  COALESCE((SELECT jsonb_agg(g.project_id::text) FROM project_device_grants g WHERE g.device_id=d.id AND g.revoked_at IS NULL),'[]'::jsonb),d.last_seen_at,d.revoked_at FROM devices d`

func (s *Store) DeviceByTokenHash(ctx context.Context, hash string) (workspacedomain.Device, error) {
	var tenantID, deviceID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,device_id FROM contentcloud_lookup_device_token($1)`, hash).Scan(&tenantID, &deviceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return workspacedomain.Device{}, fault.NotFound("设备")
		}
		return workspacedomain.Device{}, err
	}
	var result workspacedomain.Device
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanDevice(tx.QueryRow(ctx, deviceSelect+` WHERE d.tenant_id=$1 AND d.id=$2 AND d.revoked_at IS NULL`, tenantID, deviceID))
		result = v
		return dbError(err)
	})
	return result, err
}

func (s *Store) Devices(ctx context.Context, tenantID, projectID string) ([]workspacedomain.Device, error) {
	out := []workspacedomain.Device{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := deviceSelect + ` WHERE d.tenant_id=$1`
		args := []any{tenantID}
		if projectID != "" {
			query += ` AND EXISTS(SELECT 1 FROM project_device_grants g WHERE g.device_id=d.id AND g.project_id=$2 AND g.revoked_at IS NULL)`
			args = append(args, projectID)
		}
		query += ` ORDER BY d.last_seen_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanDevice(rows)
			if err != nil {
				return err
			}
			v.TokenHash = ""
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) Device(ctx context.Context, tenantID, id string) (workspacedomain.Device, error) {
	var result workspacedomain.Device
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanDevice(tx.QueryRow(ctx, deviceSelect+` WHERE d.tenant_id=$1 AND d.id=$2`, tenantID, id))
		result = v
		return dbError(err)
	})
	result.TokenHash = ""
	return result, err
}

func (s *Store) GrantDeviceProject(ctx context.Context, tenantID, projectID, deviceID, grantedBy string, now time.Time) error {
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL)`, tenantID, deviceID).Scan(&exists); err != nil || !exists {
			return fault.NotFound("设备")
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_projects WHERE tenant_id=$1 AND id=$2)`, tenantID, projectID).Scan(&exists); err != nil || !exists {
			return fault.NotFound("项目")
		}
		_, err := tx.Exec(ctx, `INSERT INTO project_device_grants(tenant_id,project_id,device_id,granted_by,granted_at,revoked_at) VALUES($1,$2,$3,$4,$5,NULL) ON CONFLICT (tenant_id,project_id,device_id) DO UPDATE SET granted_by=EXCLUDED.granted_by,granted_at=EXCLUDED.granted_at,revoked_at=NULL`, tenantID, projectID, deviceID, grantedBy, now)
		return dbError(err)
	})
}

func (s *Store) RevokeDeviceProject(ctx context.Context, tenantID, projectID, deviceID string, now time.Time) error {
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE project_device_grants SET revoked_at=$4 WHERE tenant_id=$1 AND project_id=$2 AND device_id=$3 AND revoked_at IS NULL`, tenantID, projectID, deviceID, now)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.NotFound("项目设备授权")
		}
		return nil
	})
}

func (s *Store) RevokeDevice(ctx context.Context, tenantID, id string, now time.Time) error {
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE devices SET revoked_at=$3 WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL`, tenantID, id, now)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return fault.NotFound("设备")
		}
		_, err = tx.Exec(ctx, `UPDATE project_device_grants SET revoked_at=$3 WHERE tenant_id=$1 AND device_id=$2 AND revoked_at IS NULL`, tenantID, id, now)
		return err
	})
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

var _ = fmt.Sprintf
