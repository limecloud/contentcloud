package application

import (
	"sync"
	"testing"

	workbenchdomain "github.com/limecloud/contentcloud/internal/experience/workbench"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

func TestWorkbenchRegistryControlPlanePersistsLifecycleAndTenantScope(t *testing.T) {
	store := memory.New()
	service := New(DependenciesFrom(store), nil)
	base := workbenchdomain.DefaultRegistry().Entries()[0]
	base.Manifest.ID = "custom-article-workbench"
	base.Manifest.Version = "2.0.0"
	base.Status = "draft"
	base.TenantIDs = []string{"tenant-a"}
	created, err := service.Operations.RegisterWorkbench(t.Context(), Actor{PlatformAdmin: true, UserID: "operator"}, RegisterWorkbenchInput{
		Manifest: base.Manifest, Status: base.Status, TemplateAliases: base.TemplateAliases, TenantIDs: base.TenantIDs,
	}, "request-1")
	if err != nil {
		t.Fatalf("RegisterWorkbench() error = %v", err)
	}
	if created.Digest == "" || created.Status != "draft" {
		t.Fatalf("unexpected created entry: %#v", created)
	}
	view, err := service.Operations.WorkbenchRegistry(t.Context(), Actor{PlatformAdmin: true})
	if err != nil {
		t.Fatalf("WorkbenchRegistry() error = %v", err)
	}
	if len(view.Entries) != len(workbenchdomain.DefaultRegistry().Entries())+1 {
		t.Fatalf("registry did not merge persisted entry: got %d entries", len(view.Entries))
	}
	published, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true, UserID: "operator"}, created.Manifest.ID, created.Manifest.Version, UpdateWorkbenchStateInput{Status: "published", TenantIDs: []string{"tenant-b"}}, "request-2")
	if err != nil {
		t.Fatalf("UpdateWorkbenchState() error = %v", err)
	}
	if published.Status != "published" || len(published.TenantIDs) != 1 || published.TenantIDs[0] != "tenant-b" {
		t.Fatalf("unexpected published entry: %#v", published)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, created.Manifest.ID, created.Manifest.Version, UpdateWorkbenchStateInput{Status: "published", TenantIDs: []string{"tenant-b"}}, "request-3"); err != nil {
		t.Fatalf("idempotent state update should succeed: %v", err)
	}
}

func TestWorkbenchRegistryControlPlaneRequiresPlatformAdmin(t *testing.T) {
	service := New(DependenciesFrom(memory.New()), nil)
	if _, err := service.Operations.WorkbenchRegistry(t.Context(), Actor{}); err == nil {
		t.Fatal("WorkbenchRegistry() allowed a non-platform actor")
	}
}

func TestWorkbenchRegistryPublicationRequiresApprovedTemplateAndUniqueScope(t *testing.T) {
	store := memory.New()
	service := New(DependenciesFrom(store), nil)
	base := workbenchdomain.DefaultRegistry().Entries()[0]
	base.Manifest.ID = "unapproved-video-workbench"
	base.Manifest.Version = "2.0.0"
	base.Manifest.Experience.TemplateID = "future-template"
	created, err := service.Operations.RegisterWorkbench(t.Context(), Actor{PlatformAdmin: true, UserID: "operator"}, RegisterWorkbenchInput{Manifest: base.Manifest}, "request-unknown")
	if err != nil {
		t.Fatalf("draft with an unapproved template should be registerable: %v", err)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, created.Manifest.ID, created.Manifest.Version, UpdateWorkbenchStateInput{Status: "published"}, "request-publish"); err == nil {
		t.Fatal("publication with an unapproved template was accepted")
	}

	base = workbenchdomain.DefaultRegistry().Entries()[0]
	base.Manifest.ID = "duplicate-video-workbench"
	base.Manifest.Version = "1.0.0"
	created, err = service.Operations.RegisterWorkbench(t.Context(), Actor{PlatformAdmin: true}, RegisterWorkbenchInput{Manifest: base.Manifest}, "request-duplicate")
	if err != nil {
		t.Fatalf("register duplicate scope candidate: %v", err)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, created.Manifest.ID, created.Manifest.Version, UpdateWorkbenchStateInput{Status: "published"}, "request-duplicate-publish"); err == nil {
		t.Fatal("second platform default for the same content type was accepted")
	}
}

func TestWorkbenchRegistryRegistrationCannotBypassPublicationGate(t *testing.T) {
	service := New(DependenciesFrom(memory.New()), nil)
	base := workbenchdomain.DefaultRegistry().Entries()[0]
	base.Manifest.ID = "direct-publish-workbench"
	base.Manifest.Version = "2.0.0"
	base.Manifest.Experience.TemplateID = "unapproved-template"
	if _, err := service.Operations.RegisterWorkbench(t.Context(), Actor{PlatformAdmin: true}, RegisterWorkbenchInput{Manifest: base.Manifest, Status: "published"}, "request-direct-publish"); err == nil {
		t.Fatal("RegisterWorkbench() allowed direct publication without the publication gate")
	}
}

func TestWorkbenchRegistryRegistrationCannotShadowFirstPartyVersion(t *testing.T) {
	service := New(DependenciesFrom(memory.New()), nil)
	base := workbenchdomain.DefaultRegistry().Entries()[0]
	originalName := base.Manifest.Name
	base.Manifest.Name = "尝试覆盖首方工作台"
	if _, err := service.Operations.RegisterWorkbench(t.Context(), Actor{PlatformAdmin: true}, RegisterWorkbenchInput{Manifest: base.Manifest}, "request-shadow"); err == nil {
		t.Fatal("RegisterWorkbench() allowed a persisted draft to shadow a first-party id and version")
	}
	view, err := service.Operations.WorkbenchRegistry(t.Context(), Actor{PlatformAdmin: true})
	if err != nil {
		t.Fatalf("WorkbenchRegistry() error = %v", err)
	}
	for _, entry := range view.Entries {
		if entry.String() == base.String() && entry.Manifest.Name != originalName {
			t.Fatalf("first-party version was shadowed: %#v", entry)
		}
	}
}

func TestWorkbenchRegistryLifecycleSeparatesRetirementFromSecurityRevocation(t *testing.T) {
	store := memory.New()
	service := New(DependenciesFrom(store), nil)
	base := workbenchdomain.DefaultRegistry().Entries()[1]
	base.Manifest.ID = "lifecycle-article-workbench"
	base.Manifest.Version = "2.0.0"
	created, err := service.Operations.RegisterWorkbench(t.Context(), Actor{PlatformAdmin: true}, RegisterWorkbenchInput{Manifest: base.Manifest, TenantIDs: []string{"tenant-a"}}, "create")
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, created.Manifest.ID, created.Manifest.Version, UpdateWorkbenchStateInput{Status: "published", TenantIDs: []string{"tenant-a"}}, "publish")
	if err != nil {
		t.Fatal(err)
	}
	retired, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, published.Manifest.ID, published.Manifest.Version, UpdateWorkbenchStateInput{Status: "retired", TenantIDs: published.TenantIDs}, "retire")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, retired.Manifest.ID, retired.Manifest.Version, UpdateWorkbenchStateInput{Status: "published", TenantIDs: retired.TenantIDs}, "restore"); err != nil {
		t.Fatalf("retired version should be restorable through publication gate: %v", err)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, retired.Manifest.ID, retired.Manifest.Version, UpdateWorkbenchStateInput{Status: "revoked", TenantIDs: retired.TenantIDs}, "revoke-without-reason"); err == nil {
		t.Fatal("security revocation without a reason was accepted")
	}
	revoked, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, retired.Manifest.ID, retired.Manifest.Version, UpdateWorkbenchStateInput{Status: "revoked", TenantIDs: retired.TenantIDs, Reason: "签名或来源校验失败"}, "revoke")
	if err != nil {
		t.Fatal(err)
	}
	if revoked.LifecycleReason != "签名或来源校验失败" {
		t.Fatalf("revocation reason was not persisted: %#v", revoked)
	}
	idempotent, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, revoked.Manifest.ID, revoked.Manifest.Version, UpdateWorkbenchStateInput{Status: "revoked", TenantIDs: []string{"tenant-a", "tenant-a"}, Reason: " 签名或来源校验失败 "}, "revoke-idempotent")
	if err != nil || idempotent.LifecycleReason != revoked.LifecycleReason {
		t.Fatalf("identical revocation retry should be idempotent: entry=%#v err=%v", idempotent, err)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, revoked.Manifest.ID, revoked.Manifest.Version, UpdateWorkbenchStateInput{Status: "revoked", TenantIDs: revoked.TenantIDs, Reason: "新的撤销原因"}, "rewrite-reason"); err == nil {
		t.Fatal("security revocation reason was overwritten")
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, revoked.Manifest.ID, revoked.Manifest.Version, UpdateWorkbenchStateInput{Status: "revoked", TenantIDs: []string{"tenant-b"}, Reason: revoked.LifecycleReason}, "rewrite-scope"); err == nil {
		t.Fatal("security revocation tenant scope was overwritten")
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, revoked.Manifest.ID, revoked.Manifest.Version, UpdateWorkbenchStateInput{Status: "published", TenantIDs: revoked.TenantIDs}, "restore-revoked"); err == nil {
		t.Fatal("security-revoked version was restored")
	}
}

func TestWorkbenchRegistryRejectsBackwardDraftTransition(t *testing.T) {
	store := memory.New()
	service := New(DependenciesFrom(store), nil)
	base := workbenchdomain.DefaultRegistry().Entries()[2]
	base.Manifest.ID = "state-commerce-workbench"
	base.Manifest.Version = "2.0.0"
	created, err := service.Operations.RegisterWorkbench(t.Context(), Actor{PlatformAdmin: true}, RegisterWorkbenchInput{Manifest: base.Manifest, TenantIDs: []string{"tenant-a"}}, "create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, created.Manifest.ID, created.Manifest.Version, UpdateWorkbenchStateInput{Status: "published", TenantIDs: created.TenantIDs}, "publish"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, created.Manifest.ID, created.Manifest.Version, UpdateWorkbenchStateInput{Status: "draft", TenantIDs: created.TenantIDs}, "back-to-draft"); err == nil {
		t.Fatal("published version returned to draft")
	}
}

func TestWorkbenchRegistryConcurrentPublicationsKeepSingleScopeWinner(t *testing.T) {
	store := memory.New()
	service := New(DependenciesFrom(store), nil)
	base := workbenchdomain.DefaultRegistry().Entries()[1]
	entries := make([]workbenchdomain.Entry, 0, 2)
	for _, id := range []string{"concurrent-article-a", "concurrent-article-b"} {
		manifest := base.Manifest
		manifest.ID = id
		manifest.Version = "3.0.0"
		created, err := service.Operations.RegisterWorkbench(t.Context(), Actor{PlatformAdmin: true}, RegisterWorkbenchInput{Manifest: manifest, TenantIDs: []string{"tenant-concurrent"}}, "register-"+id)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, created)
	}

	start := make(chan struct{})
	results := make(chan error, len(entries))
	var wait sync.WaitGroup
	for _, entry := range entries {
		entry := entry
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := service.Operations.UpdateWorkbenchState(t.Context(), Actor{PlatformAdmin: true}, entry.Manifest.ID, entry.Manifest.Version, UpdateWorkbenchStateInput{Status: "published", TenantIDs: entry.TenantIDs}, "publish-"+entry.Manifest.ID)
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent publications succeeded %d times, want exactly one", successes)
	}
	view, err := service.Operations.WorkbenchRegistry(t.Context(), Actor{PlatformAdmin: true})
	if err != nil {
		t.Fatal(err)
	}
	published := 0
	for _, entry := range view.Entries {
		if entry.Status == "published" && len(entry.TenantIDs) == 1 && entry.TenantIDs[0] == "tenant-concurrent" && workbenchdomain.SharesContentType(entry.Manifest.ContentTypes, base.Manifest.ContentTypes) {
			published++
		}
	}
	if published != 1 {
		t.Fatalf("registry contains %d published tenant winners, want exactly one", published)
	}
}
