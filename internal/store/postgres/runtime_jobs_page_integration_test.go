package postgres_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
)

func TestRuntimePostgresJobRunsPageSupportsEmptyAndUUIDProjectFilters(t *testing.T) {
	databaseURL := os.Getenv("CONTENTCLOUD_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTENTCLOUD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := storepg.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)

	service := app.New(store, slog.Default())
	suffix := domain.NewID()
	session, err := service.Register(ctx, fmt.Sprintf("runtime-page-%s@example.com", suffix), "long-enough-password", "Runtime Page", "Runtime Page "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id=$1`, actor.TenantID)
	}()
	projectOne, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Page Brand One", ProductName: "Page Product One"}, "")
	if err != nil {
		t.Fatal(err)
	}
	projectTwo, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Page Brand Two", ProductName: "Page Product Two"}, "")
	if err != nil {
		t.Fatal(err)
	}

	sop := domain.SOPVersion{
		ID: "runtime-page-sop-v1", TenantID: actor.TenantID, SOPID: "runtime-page-sop", Version: 1,
		SchemaVersion: domain.SOPSchemaVersion, Name: "Runtime Page", Status: "published", DefaultExecutionMode: "agent",
		Stages: []domain.StageDefinition{{ID: "page-node", Name: "Page Node", Order: 10, OutputSchema: "contentcloud.runtime-page/1.0", ExecutionModes: []string{"agent"}}},
	}
	start := func(projectID, key string) {
		t.Helper()
		_, startErr := service.Runtime().Start(ctx, contentruntime.StartInput{
			TenantID: actor.TenantID, ProjectID: projectID, WorkTaskID: "runtime-page-task-" + key,
			SOP: sop, BindingDigest: "sha256:" + strings.Repeat("a", 64), InputDigest: "sha256:" + strings.Repeat("b", 64),
			RuntimePolicyID: "runtime-policy/runtime-page", ContractMajor: 1, ContractMinor: 0,
			CreatedBy: actor.UserID, IdempotencyKey: "runtime-page-" + key,
		})
		if startErr != nil {
			t.Fatal(startErr)
		}
	}
	start(projectOne.ID, "one")
	start(projectTwo.ID, "two")

	all, hasMore, err := store.JobRunsPage(ctx, actor.TenantID, "", "", 0, 10)
	if err != nil {
		t.Fatalf("empty project filter must not fail: %v", err)
	}
	if hasMore || len(all) != 2 {
		t.Fatalf("unexpected unfiltered page: count=%d has_more=%v", len(all), hasMore)
	}

	filtered, hasMore, err := store.JobRunsPage(ctx, actor.TenantID, projectOne.ID, "", 0, 10)
	if err != nil {
		t.Fatalf("UUID project filter failed: %v", err)
	}
	if hasMore || len(filtered) != 1 || filtered[0].ProjectID != projectOne.ID {
		t.Fatalf("unexpected project-filtered page: jobs=%#v has_more=%v", filtered, hasMore)
	}
}
