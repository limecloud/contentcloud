package application

import (
	"context"
	"errors"
	"time"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

// persistArtifactObject coordinates speculative Blob persistence with the
// existing Artifact fact. A failed fact write must not leave an unattached
// object in Blob storage.
func (s *serviceScope) persistArtifactObject(ctx context.Context, artifact deliverydomain.Artifact, body []byte) error {
	if err := s.blobs.Put(ctx, artifact.ObjectKey, body); err != nil {
		return cleanupSpeculativeObjects(ctx, s.blobs, []string{artifact.ObjectKey}, err)
	}
	if err := s.artifacts.CreateArtifact(ctx, artifact); err != nil {
		return cleanupSpeculativeObjects(ctx, s.blobs, []string{artifact.ObjectKey}, err)
	}
	return nil
}

// cleanupSpeculativeObjects preserves the original business error when
// cleanup succeeds and upgrades it with an actionable object list when the
// storage cleanup itself fails.
func cleanupSpeculativeObjects(ctx context.Context, store blob.Store, keys []string, cause error) error {
	deleter, ok := store.(blob.DeleteStore)
	if !ok {
		return cause
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	failed := map[string]string{}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if err := deleter.Delete(cleanupCtx, key); err != nil && !errors.Is(err, blob.ErrNotFound) {
			failed[key] = err.Error()
		}
	}
	if len(failed) == 0 {
		return cause
	}
	if domainErr, ok := cause.(*fault.Error); ok {
		copy := *domainErr
		copy.Retryable = true
		copy.Message += "；临时对象清理失败"
		copy.Details = map[string]any{"cleanup_errors": failed, "cause": cause.Error()}
		return &copy
	}
	return &fault.Error{Type: "storage", Subtype: "consistency", Code: "ARTIFACT_BLOB_CLEANUP_FAILED", Message: "业务事实写入失败且临时对象清理失败", Retryable: true, Hint: "根据 cleanup_errors 清理孤立对象后重试", Details: map[string]any{"cleanup_errors": failed, "cause": cause.Error()}, ExitCode: 6}
}
