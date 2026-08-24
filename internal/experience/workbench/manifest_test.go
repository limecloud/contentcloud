package workbench

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeManifestAcceptsDeclarativeWorkbench(t *testing.T) {
	manifest, err := DecodeManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if manifest.ID != "contentcloud-workbench-marketing-video" || manifest.UI.Layout != "stage-canvas-context" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
}

func TestDecodeManifestRejectsExecutableOrUnknownFields(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field":       strings.Replace(validManifest, `"ui":`, `"entry":"./bundle.js","ui":`, 1),
		"unapproved renderer": strings.Replace(validManifest, `"renderer":"approved"`, `"renderer":"remote-script"`, 1),
		"duplicate stage":     strings.Replace(validManifest, `{"id":"script"`, `{"id":"brief"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeManifest([]byte(body)); err == nil {
				t.Fatal("DecodeManifest() error = nil, want rejection")
			}
		})
	}
}

func TestDecodeManifestRejectsInvalidVersionAndContentType(t *testing.T) {
	body := strings.Replace(validManifest, `"version":"1.0.0"`, `"version":"1.0"`, 1)
	body = strings.Replace(body, `"content_types":["marketing_video"]`, `"content_types":[]`, 1)
	if _, err := DecodeManifest([]byte(body)); err == nil {
		t.Fatal("DecodeManifest() error = nil, want rejection")
	}
}

func TestDecodeManifestRejectsUnapprovedPlatformAction(t *testing.T) {
	body := strings.Replace(validManifest, `"primary_action":"save_brief"`, `"primary_action":"invent_state_transition"`, 1)
	if _, err := DecodeManifest([]byte(body)); err == nil {
		t.Fatal("DecodeManifest() error = nil for an unapproved platform action, want rejection")
	}
}

func TestDecodeManifestRejectsUnapprovedThemeAndIcon(t *testing.T) {
	for name, body := range map[string]string{
		"theme": strings.Replace(validManifest, `"theme":"signal-blue"`, `"theme":"remote-theme"`, 1),
		"icon":  strings.Replace(validManifest, `"icon":"video"`, `"icon":"custom-svg"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeManifest([]byte(body)); err == nil {
				t.Fatal("DecodeManifest() accepted UI vocabulary outside the approved contract")
			}
		})
	}
}

func TestDecodeManifestRejectsTooManyContentTypes(t *testing.T) {
	body := strings.Replace(validManifest, `"content_types":["marketing_video"]`, `"content_types":["one","two","three","four","five","six","seven","eight","nine"]`, 1)
	if _, err := DecodeManifest([]byte(body)); err == nil {
		t.Fatal("DecodeManifest() error = nil for more than eight content types, want rejection")
	}
}

func TestSameTenantScopeIgnoresOrderWhitespaceAndDuplicates(t *testing.T) {
	if !SameTenantScope([]string{"tenant-b", " tenant-a", "tenant-a"}, []string{"tenant-a", "tenant-b"}) {
		t.Fatal("equivalent tenant assignments were treated as different scopes")
	}
	if SameTenantScope([]string{"tenant-a"}, []string{"tenant-b"}) {
		t.Fatal("different tenant assignments were treated as the same scope")
	}
	if SameTenantScope(nil, []string{"tenant-a"}) {
		t.Fatal("platform and tenant-specific assignments were treated as the same scope")
	}
}

func TestDecodeManifestAcceptsAndValidatesDeclarativePanels(t *testing.T) {
	body := strings.Replace(validManifest, `"stages":[`, `"panels":[{"id":"direction","title":"目标与资料","detail":"固定业务输入","tone":"source","icon":"folder","stage_ids":["brief"],"target":"start","action_label":"开始策划"}],"stages":[`, 1)
	manifest, err := DecodeManifest([]byte(body))
	if err != nil || len(manifest.UI.Panels) != 1 {
		t.Fatalf("valid panel manifest rejected: manifest=%#v err=%v", manifest, err)
	}
	invalid := strings.Replace(body, `"stage_ids":["brief"]`, `"stage_ids":["missing"]`, 1)
	if _, err := DecodeManifest([]byte(invalid)); err == nil {
		t.Fatal("panel referencing an unknown stage was accepted")
	}
}

func TestDecodeManifestRejectsTrailingJSONAndDuplicateContentTypes(t *testing.T) {
	if _, err := DecodeManifest([]byte(validManifest + validManifest)); err == nil {
		t.Fatal("DecodeManifest() error = nil for trailing JSON, want rejection")
	}
	body := strings.Replace(validManifest, `"content_types":["marketing_video"]`, `"content_types":["marketing_video","marketing_video"]`, 1)
	if _, err := DecodeManifest([]byte(body)); err == nil {
		t.Fatal("DecodeManifest() error = nil for duplicate content type, want rejection")
	}
}

func TestRegistryResolvesPublishedManifestByTemplateAndTenant(t *testing.T) {
	registry := DefaultRegistry()
	entry, err := registry.Resolve("marketing_video", "builtin-sop-marketing-video", "tenant-1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if entry.Manifest.ID != "contentcloud-workbench-marketing-video" || entry.Digest == "" || entry.Status != "published" {
		t.Fatalf("unexpected registry entry: %+v", entry)
	}
	entry.Manifest.ContentTypes[0] = "mutated"
	copyEntry, err := registry.Resolve("marketing_video", "ip_persona_marketing_video", "tenant-1")
	if err != nil {
		t.Fatalf("Resolve() after mutation error = %v", err)
	}
	if copyEntry.Manifest.ContentTypes[0] != "marketing_video" {
		t.Fatal("Resolve() returned mutable registry state")
	}
	if _, err := registry.Resolve("marketing-video", "ip_persona_marketing_video", "tenant-1"); err != nil {
		t.Fatalf("Resolve() should accept the canonical hyphen alias: %v", err)
	}
}

func TestRegistryHonorsTenantAllowListAndPublishedStatus(t *testing.T) {
	manifest, err := DecodeManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	registry, err := NewRegistry([]Entry{{Manifest: manifest, Status: "draft", TenantIDs: []string{"tenant-1"}}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if _, err := registry.Resolve("marketing_video", manifest.Experience.TemplateID, "tenant-1"); err == nil {
		t.Fatal("Resolve() allowed a draft entry")
	}
	registry, err = NewRegistry([]Entry{{Manifest: manifest, Status: "published", TenantIDs: []string{"tenant-1"}}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if _, err := registry.Resolve("marketing_video", manifest.Experience.TemplateID, "tenant-2"); err == nil {
		t.Fatal("Resolve() ignored tenant allow-list")
	}
}

func TestRegistryPrefersTenantScopedWorkbenchOverPlatformDefault(t *testing.T) {
	defaultManifest, err := DecodeManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	scoped := defaultManifest
	scoped.ID = "tenant-video-workbench"
	scoped.Version = "1.0.0"
	registry, err := NewRegistry([]Entry{
		{Manifest: defaultManifest, Status: "published"},
		{Manifest: scoped, Status: "published", TenantIDs: []string{"tenant-1"}},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	entry, err := registry.Resolve("marketing_video", defaultManifest.Experience.TemplateID, "tenant-1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if entry.Manifest.ID != scoped.ID {
		t.Fatalf("Resolve() selected %q, want tenant-scoped %q", entry.Manifest.ID, scoped.ID)
	}
	entry, err = registry.Resolve("marketing_video", defaultManifest.Experience.TemplateID, "tenant-2")
	if err != nil {
		t.Fatalf("Resolve() for default tenant error = %v", err)
	}
	if entry.Manifest.ID != defaultManifest.ID {
		t.Fatalf("Resolve() selected %q, want platform default %q", entry.Manifest.ID, defaultManifest.ID)
	}
}

func TestRegistryPrefersHigherPublishedVersion(t *testing.T) {
	older, err := DecodeManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	newer := older
	newer.Version = "2.0.0"
	registry, err := NewRegistry([]Entry{{Manifest: older, Status: "published"}, {Manifest: newer, Status: "published"}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	entry, err := registry.Resolve("marketing_video", older.Experience.TemplateID, "tenant-1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if entry.Manifest.Version != newer.Version {
		t.Fatalf("Resolve() selected version %s, want %s", entry.Manifest.Version, newer.Version)
	}
}

func TestRegistryResolvesPinnedVersionAfterRetirement(t *testing.T) {
	manifest, err := DecodeManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	registry, err := NewRegistry([]Entry{{Manifest: manifest, Status: "retired"}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	entry := registry.Entries()[0]
	resolved, err := registry.ResolveVersion(entry.Manifest.ID, entry.Manifest.Version, entry.Digest)
	if err != nil {
		t.Fatalf("ResolveVersion() error = %v", err)
	}
	if resolved.Status != "retired" || resolved.Digest != entry.Digest {
		t.Fatalf("unexpected pinned entry: %#v", resolved)
	}
	if _, err := registry.ResolveVersion(entry.Manifest.ID, entry.Manifest.Version, "sha256:"+strings.Repeat("f", 64)); err == nil {
		t.Fatal("ResolveVersion() accepted digest drift")
	}
	revoked, err := NewRegistry([]Entry{{Manifest: manifest, Status: "revoked", LifecycleReason: "签名验证失败"}})
	if err != nil {
		t.Fatalf("NewRegistry() revoked entry error = %v", err)
	}
	if _, err := revoked.ResolveVersion(entry.Manifest.ID, entry.Manifest.Version, revoked.Entries()[0].Digest); err == nil {
		t.Fatal("ResolveVersion() rendered a security-revoked workbench")
	}
}

func TestRegistryRejectsProvidedDigestDrift(t *testing.T) {
	manifest, err := DecodeManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if _, err := NewRegistry([]Entry{{Manifest: manifest, Status: "draft", Digest: "sha256:" + strings.Repeat("f", 64)}}); err == nil {
		t.Fatal("NewRegistry() accepted a valid-looking digest for different manifest content")
	}
}

func TestRegistryEntryJSONKeepsOptionalCollectionsAsArrays(t *testing.T) {
	entry := DefaultRegistry().Entries()[0]
	body, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, field := range []string{"template_aliases", "tenant_ids"} {
		value, ok := payload[field]
		if !ok {
			t.Fatalf("%s is missing from the registry entry JSON", field)
		}
		var items []string
		if err := json.Unmarshal(value, &items); err != nil {
			t.Fatalf("%s is not an array: %v", field, err)
		}
		if items == nil {
			t.Fatalf("%s is null, want an empty array", field)
		}
	}
}

const validManifest = `{
  "$schema":"https://contentcloud.run/schemas/workbench-plugin/1.0.0/workbench-plugin.schema.json",
  "id":"contentcloud-workbench-marketing-video",
  "version":"1.0.0",
  "name":"视频生产工作台",
  "content_types":["marketing_video"],
  "experience":{"template_id":"ip_persona_marketing_video"},
  "ui":{
    "renderer":"approved",
    "layout":"stage-canvas-context",
    "density":"comfortable",
    "theme":"signal-blue",
    "navigation":[{"id":"overview","label":"视频首页","icon":"video"}],
    "stages":[
      {"id":"brief","label":"目标与资料","outcome":"确定视频目标","primary_action":"save_brief"},
      {"id":"script","label":"剧本","outcome":"确认营销剧本","primary_action":"submit_script"}
    ]
  }
}`
