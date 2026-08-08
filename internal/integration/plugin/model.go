package plugin

import "encoding/json"

const (
	SpecVersion         = "1.0.0"
	ManifestSchemaURL   = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	MCPSchemaURL        = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
	ExtensionNamespace  = "run.zhongcao.contentcloud"
	ClaimsSchemaVersion = "contentcloud.plugin-claims/1.0"
)

type DiagnosticLevel string

const (
	DiagnosticWarning     DiagnosticLevel = "warning"
	DiagnosticError       DiagnosticLevel = "error"
	DiagnosticUnsupported DiagnosticLevel = "unsupported"
)

type Diagnostic struct {
	Level     DiagnosticLevel `json:"level"`
	Code      string          `json:"code"`
	Path      string          `json:"path,omitempty"`
	Component string          `json:"component,omitempty"`
	Message   string          `json:"message"`
}

type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

type Manifest struct {
	Schema      string                     `json:"$schema"`
	Name        string                     `json:"name"`
	Version     string                     `json:"version,omitempty"`
	Description string                     `json:"description,omitempty"`
	Author      *Author                    `json:"author,omitempty"`
	Homepage    string                     `json:"homepage,omitempty"`
	Repository  string                     `json:"repository,omitempty"`
	License     string                     `json:"license,omitempty"`
	Keywords    []string                   `json:"keywords,omitempty"`
	Extensions  map[string]json.RawMessage `json:"extensions,omitempty"`
}

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Digest      string `json:"digest"`
}

type MCPServer struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	CWD       string            `json:"cwd,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Supported bool              `json:"supported"`
}

type Package struct {
	Root         string       `json:"root"`
	SpecVersion  string       `json:"spec_version"`
	Manifest     Manifest     `json:"manifest"`
	Skills       []Skill      `json:"skills"`
	MCPServers   []MCPServer  `json:"mcp_servers"`
	Claims       *Claims      `json:"claims,omitempty"`
	ClaimsDigest string       `json:"claims_digest,omitempty"`
	Digest       string       `json:"digest"`
	Files        int          `json:"files"`
	Bytes        int64        `json:"bytes"`
	Diagnostics  []Diagnostic `json:"diagnostics"`
}

type Limits struct {
	MaxFiles     int
	MaxFileBytes int64
	MaxPackBytes int64
	MaxDepth     int
}

func DefaultLimits() Limits {
	return Limits{
		MaxFiles:     4096,
		MaxFileBytes: 8 << 20,
		MaxPackBytes: 64 << 20,
		MaxDepth:     32,
	}
}
