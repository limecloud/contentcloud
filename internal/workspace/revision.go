package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	WorkspaceRevisionSchemaVersion = "contentcloud.workspace-revision/1.0"
	WorkspaceUploadChunkSize       = 4 * 1024 * 1024
	WorkspaceUploadMaxFileSize     = 512 * 1024 * 1024
)

var workspaceRevisionRoots = map[string]struct{}{
	"10-context": {}, "20-sources": {}, "30-knowledge": {}, "40-work": {},
	"50-production": {}, "60-delivery": {}, "70-results": {}, "90-archive": {},
}

// WorkspaceRevision is the immutable cloud checkpoint for one observed local
// workspace. File bodies are uploaded and referenced separately; this record
// only owns revision identity, ordering and digest fences.
type WorkspaceRevision struct {
	ID               string                  `json:"id"`
	TenantID         string                  `json:"tenant_id"`
	ProjectID        string                  `json:"project_id"`
	WorkspaceID      string                  `json:"workspace_id"`
	DeviceID         string                  `json:"device_id"`
	SchemaVersion    string                  `json:"schema_version"`
	RevisionNo       int64                   `json:"revision_no"`
	BaseRevisionID   string                  `json:"base_revision_id"`
	ContentDigest    string                  `json:"content_digest"`
	Files            []WorkspaceRevisionFile `json:"files"`
	ClientMutationID string                  `json:"client_mutation_id"`
	IdempotencyKey   string                  `json:"idempotency_key"`
	CreatedAt        time.Time               `json:"created_at"`
}

type WorkspaceRevisionFile struct {
	Ref      string `json:"ref"`
	Digest   string `json:"digest"`
	ByteSize int64  `json:"byte_size"`
}

func ValidWorkspaceRevisionFileRef(ref string) bool {
	if ref == "" || strings.Contains(ref, "\\") || strings.ContainsRune(ref, '\x00') || strings.HasPrefix(ref, "/") || path.Clean(ref) != ref {
		return false
	}
	root, _, _ := strings.Cut(ref, "/")
	_, ok := workspaceRevisionRoots[root]
	return ok && root != ref
}

func WorkspaceContentDigest(files []WorkspaceRevisionFile) string {
	ordered := append([]WorkspaceRevisionFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Ref < ordered[j].Ref })
	hash := sha256.New()
	for _, file := range ordered {
		_, _ = io.WriteString(hash, strings.Join([]string{file.Ref, fmt.Sprint(file.ByteSize), strings.TrimPrefix(file.Digest, "sha256:")}, "\x00")+"\n")
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

type WorkspaceUploadSession struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	WorkspaceID    string    `json:"workspace_id"`
	DeviceID       string    `json:"device_id"`
	Ref            string    `json:"ref"`
	ContentDigest  string    `json:"content_digest"`
	ByteSize       int64     `json:"byte_size"`
	ChunkSize      int64     `json:"chunk_size"`
	PartCount      int       `json:"part_count"`
	State          string    `json:"state"`
	ObjectKey      string    `json:"-"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type WorkspaceUploadPart struct {
	SessionID string    `json:"session_id"`
	PartNo    int       `json:"part_no"`
	Digest    string    `json:"digest"`
	ByteSize  int64     `json:"byte_size"`
	ObjectKey string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type WorkspaceObject struct {
	TenantID      string    `json:"tenant_id"`
	ProjectID     string    `json:"project_id"`
	ContentDigest string    `json:"content_digest"`
	ByteSize      int64     `json:"byte_size"`
	ObjectKey     string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
}
