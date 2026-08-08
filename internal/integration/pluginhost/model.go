package pluginhost

import (
	"context"
	"encoding/json"
	"time"

	"github.com/limecloud/contentcloud/internal/integration/plugin"
)

const SchemaVersion = "contentcloud.pluginhost/1.0"

type HostID string

const (
	HostCodex  HostID = "codex"
	HostClaude HostID = "claude"
)

type Status string

const (
	StatusAbsent          Status = "absent"
	StatusStaged          Status = "staged"
	StatusInstalled       Status = "installed"
	StatusReady           Status = "ready"
	StatusRepairRequired  Status = "repair_required"
	StatusUnsupportedHost Status = "unsupported_host"
	StatusBlocked         Status = "blocked"
	StatusRemoved         Status = "removed"
	StatusInstallFailed   Status = "install_failed"
)

type ReleaseRef struct {
	PluginID string `json:"plugin_id"`
	Version  string `json:"version"`
	Digest   string `json:"digest"`
}

type ComponentRef struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Digest string `json:"digest,omitempty"`
}

type Capabilities struct {
	PluginDirectoryInstall bool     `json:"plugin_directory_install"`
	Skills                 bool     `json:"skills"`
	MCPStdio               bool     `json:"mcp_stdio"`
	MCPStreamableHTTP      bool     `json:"mcp_streamable_http"`
	NewSessionRequired     bool     `json:"new_session_required"`
	AtomicInstall          bool     `json:"atomic_install"`
	Rollback               bool     `json:"rollback"`
	NativeExtensions       []string `json:"native_extensions,omitempty"`
}

type HostTarget struct {
	Release ReleaseRef
	Skills  []string
	MCP     []MCPTarget
}

type MCPTarget struct {
	Name string
	Type string
}

func TargetFromPackage(pkg plugin.Package) HostTarget {
	target := HostTarget{
		Release: ReleaseRef{PluginID: pkg.Manifest.Name, Version: pkg.Manifest.Version, Digest: pkg.Digest},
		Skills:  make([]string, 0, len(pkg.Skills)),
		MCP:     make([]MCPTarget, 0, len(pkg.MCPServers)),
	}
	for _, skill := range pkg.Skills {
		target.Skills = append(target.Skills, skill.Name)
	}
	for _, server := range pkg.MCPServers {
		target.MCP = append(target.MCP, MCPTarget{Name: server.Name, Type: server.Type})
	}
	return target
}

type ComponentState struct {
	Component ComponentRef `json:"component"`
	Status    Status       `json:"status"`
	Reason    string       `json:"reason,omitempty"`
}

type State struct {
	SchemaVersion string           `json:"schema_version"`
	HostID        HostID           `json:"host_id"`
	Status        Status           `json:"status"`
	Release       *ReleaseRef      `json:"release,omitempty"`
	Generation    string           `json:"generation,omitempty"`
	Capabilities  Capabilities     `json:"capabilities"`
	Components    []ComponentState `json:"components"`
	Reason        string           `json:"reason,omitempty"`
}

type Action struct {
	Kind      string `json:"kind"`
	Component string `json:"component"`
	Name      string `json:"name"`
	Reason    string `json:"reason"`
}

type Plan struct {
	SchemaVersion        string     `json:"schema_version"`
	Mode                 string     `json:"mode"`
	HostID               HostID     `json:"host_id"`
	Release              ReleaseRef `json:"release"`
	ObservedGeneration   string     `json:"observed_generation,omitempty"`
	State                Status     `json:"state"`
	Actions              []Action   `json:"actions"`
	BlockingReasons      []string   `json:"blocking_reasons,omitempty"`
	RequiresConfirmation bool       `json:"requires_confirmation"`
	PlanDigest           string     `json:"plan_digest"`
}

type InstalledComponent struct {
	ComponentRef
	InstalledPath string `json:"installed_path,omitempty"`
	Transport     string `json:"transport,omitempty"`
}

type Receipt struct {
	SchemaVersion     string               `json:"schema_version"`
	InstallationID    string               `json:"installation_id"`
	HostID            HostID               `json:"host_id"`
	Release           ReleaseRef           `json:"release"`
	PlanDigest        string               `json:"plan_digest"`
	Status            Status               `json:"status"`
	Installed         []InstalledComponent `json:"installed_components,omitempty"`
	PreviousReceipt   *Receipt             `json:"previous_receipt,omitempty"`
	NativeData        json.RawMessage      `json:"native_data,omitempty"`
	InstalledAt       time.Time            `json:"installed_at"`
	VerifiedAt        time.Time            `json:"verified_at"`
	RollbackReference string               `json:"rollback_reference,omitempty"`
	Diagnostics       []string             `json:"diagnostics,omitempty"`
}

type NativeApply struct {
	Target         HostTarget
	Package        plugin.Package
	PackageRoot    string
	PluginDataRoot string
	InstallationID string
	InstalledAt    time.Time
}

type NativeRemove struct {
	Target         HostTarget
	Receipt        Receipt
	PluginDataRoot string
}

type NativeChange struct {
	Data json.RawMessage
}

// NativeHost contains only host-specific paths and config materialization.
// The shared Adapter owns plan, confirmation, CAS, receipt, and lifecycle state.
type NativeHost interface {
	ID() HostID
	Capabilities(context.Context) (Capabilities, error)
	Detect(context.Context, HostTarget) (State, error)
	Apply(context.Context, NativeApply) (NativeChange, []InstalledComponent, error)
	Remove(context.Context, NativeRemove) (NativeChange, error)
	Rollback(context.Context, NativeChange) error
	Commit(context.Context, NativeChange) error
}
