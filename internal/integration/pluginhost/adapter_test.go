package pluginhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/integration/plugin"
)

type fakeNativeHost struct {
	id            HostID
	state         State
	applyChange   NativeChange
	applyErr      error
	removeChange  NativeChange
	removeErr     error
	commitErr     error
	rollbackCalls int
	commitCalls   int
}

func (f *fakeNativeHost) ID() HostID {
	return f.id
}

func (f *fakeNativeHost) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{Skills: true, MCPStdio: true, Rollback: true}, nil
}

func (f *fakeNativeHost) Detect(context.Context, HostTarget) (State, error) {
	state := f.state
	state.SchemaVersion = SchemaVersion
	state.HostID = f.id
	state.Capabilities = Capabilities{Skills: true, MCPStdio: true, Rollback: true}
	if state.Components == nil {
		state.Components = []ComponentState{}
	}
	return state, nil
}

func (f *fakeNativeHost) Apply(context.Context, NativeApply) (NativeChange, []InstalledComponent, error) {
	return f.applyChange, nil, f.applyErr
}

func (f *fakeNativeHost) Remove(context.Context, NativeRemove) (NativeChange, error) {
	return f.removeChange, f.removeErr
}

func (f *fakeNativeHost) Rollback(context.Context, NativeChange) error {
	f.rollbackCalls++
	return nil
}

func (f *fakeNativeHost) Commit(context.Context, NativeChange) error {
	f.commitCalls++
	return f.commitErr
}

func TestStoreLockSerializesAllPluginsForOneHost(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := store.Lock(HostCodex)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := store.Lock(HostCodex); err == nil {
		t.Fatal("second plugin mutation on the same host acquired a parallel lock")
	}
	claudeUnlock, err := store.Lock(HostClaude)
	if err != nil {
		t.Fatalf("independent host was unnecessarily blocked: %v", err)
	}
	claudeUnlock()
}

func TestApplyDoesNotRollbackBeforeNativeStateExists(t *testing.T) {
	pkg := testPackage(t)
	host := &fakeNativeHost{id: HostCodex, state: State{Status: StatusAbsent, Generation: "g1"}, applyErr: errors.New("inventory unavailable")}
	adapter := testAdapter(t, host)
	plan, err := adapter.Plan(t.Context(), pkg, "install")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Apply(t.Context(), pkg, plan, true); err == nil {
		t.Fatal("native apply failure was ignored")
	}
	if host.rollbackCalls != 0 {
		t.Fatalf("empty native change triggered rollback: %d", host.rollbackCalls)
	}
}

func TestApplyCommitFailureRollsBackNativeStateAndReceipt(t *testing.T) {
	pkg := testPackage(t)
	host := &fakeNativeHost{
		id:          HostCodex,
		state:       State{Status: StatusAbsent, Generation: "g1"},
		applyChange: NativeChange{Data: []byte(`{"changed":true}`)},
		commitErr:   errors.New("commit failed"),
	}
	adapter := testAdapter(t, host)
	plan, err := adapter.Plan(t.Context(), pkg, "install")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Apply(t.Context(), pkg, plan, true); err == nil {
		t.Fatal("native commit failure was ignored")
	}
	if host.rollbackCalls != 1 || host.commitCalls != 1 {
		t.Fatalf("unexpected transaction calls: rollback=%d commit=%d", host.rollbackCalls, host.commitCalls)
	}
	receipt, err := adapter.Store.LoadReceipt(HostCodex, pkg.Manifest.Name)
	if err != nil {
		t.Fatal(err)
	}
	if receipt != nil {
		t.Fatalf("failed commit left an authoritative receipt: %#v", receipt)
	}
}

func TestPlanBlocksSameVersionWithDifferentDigest(t *testing.T) {
	pkg := testPackage(t)
	host := &fakeNativeHost{id: HostCodex, state: State{Status: StatusInstalled, Generation: "g1"}}
	adapter := testAdapter(t, host)
	previous := Receipt{
		SchemaVersion: SchemaVersion,
		HostID:        HostCodex,
		Release: ReleaseRef{
			PluginID: pkg.Manifest.Name,
			Version:  pkg.Manifest.Version,
			Digest:   "sha256:" + strings.Repeat("f", 64),
		},
	}
	if err := adapter.Store.SaveReceipt(previous); err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.Plan(t.Context(), pkg, "install")
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != StatusBlocked || len(plan.BlockingReasons) != 1 || !strings.Contains(plan.BlockingReasons[0], "提升插件版本") {
		t.Fatalf("same version digest drift was not blocked: %#v", plan)
	}
}

func testAdapter(t *testing.T, host NativeHost) *Adapter {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(host, store)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func testPackage(t *testing.T) plugin.Package {
	t.Helper()
	root := t.TempDir()
	body := `{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "example-plugin",
  "version": "1.2.3",
  "description": "Plugin host adapter fixture"
}
`
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := plugin.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}
