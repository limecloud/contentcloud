package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	localconfig "github.com/limecloud/contentcloud/internal/local/config"
	localsync "github.com/limecloud/contentcloud/internal/local/sync"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	apiclient "github.com/limecloud/contentcloud/internal/transport/client"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type desktopCloudPublisher struct {
	client         *apiclient.Client
	store          *localsync.Store
	workspaceRoots map[string]string
}

func (p desktopCloudPublisher) PublishWorkspace(ctx context.Context, command localsync.PendingCommand) (localsync.CloudRevision, error) {
	for _, file := range command.Files {
		if err := p.uploadWorkspaceFile(ctx, command, file); err != nil {
			if _, ok := err.(*localsync.PublishError); ok {
				return localsync.CloudRevision{}, err
			}
			return localsync.CloudRevision{}, &localsync.PublishError{Code: "WORKSPACE_FILE_CHANGED", Conflict: true}
		}
	}
	var revision workspacedomain.WorkspaceRevision
	err := p.client.Dispatch(ctx, "desktop.workspace.publish", application.PublishWorkspaceRevisionInput{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, BaseRevision: command.BaseRevision,
		ContentDigest: command.ContentDigest, Files: command.Files, ClientMutationID: command.RequestID, IdempotencyKey: command.IdempotencyKey,
	}, &revision)
	if err != nil {
		return localsync.CloudRevision{}, classifyDesktopPublishError(err)
	}
	if revision.ID == "" || revision.ContentDigest != command.ContentDigest {
		return localsync.CloudRevision{}, &localsync.PublishError{Code: "CLOUD_REVISION_RESPONSE_INVALID"}
	}
	return localsync.CloudRevision{ID: revision.ID, ContentDigest: revision.ContentDigest}, nil
}

func (p desktopCloudPublisher) uploadWorkspaceFile(ctx context.Context, command localsync.PendingCommand, manifest workspacedomain.WorkspaceRevisionFile) error {
	root := strings.TrimSpace(p.workspaceRoots[command.ProjectID])
	if root == "" || !workspacedomain.ValidWorkspaceRevisionFileRef(manifest.Ref) {
		return &localsync.PublishError{Code: "WORKSPACE_FILE_CHANGED", Conflict: true}
	}
	filePath, err := safeWorkspaceFilePath(root, manifest.Ref)
	if err != nil {
		return &localsync.PublishError{Code: "WORKSPACE_FILE_CHANGED", Conflict: true}
	}
	file, err := os.Open(filePath)
	if err != nil {
		return &localsync.PublishError{Code: "WORKSPACE_FILE_UNREADABLE", Conflict: true}
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() != manifest.ByteSize {
		return &localsync.PublishError{Code: "WORKSPACE_FILE_CHANGED", Conflict: true}
	}
	var started application.WorkspaceUploadStartResult
	if err := p.client.Dispatch(ctx, "desktop.upload.start", application.StartWorkspaceUploadInput{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, Ref: manifest.Ref, ContentDigest: manifest.Digest, ByteSize: manifest.ByteSize,
		IdempotencyKey: command.IdempotencyKey + ":" + manifest.Ref + ":" + strings.TrimPrefix(manifest.Digest, "sha256:"),
	}, &started); err != nil {
		return classifyDesktopPublishError(err)
	}
	confirmed := make(map[int]bool, len(started.ConfirmedParts))
	for _, partNo := range started.ConfirmedParts {
		confirmed[partNo] = true
	}
	uploadState := "uploading"
	if started.Completed {
		uploadState = "completed"
	}
	if p.store != nil {
		if err := p.store.SaveUploadTransfer(ctx, localsync.UploadTransfer{ProjectID: command.ProjectID, Ref: manifest.Ref, ContentDigest: manifest.Digest, ByteSize: manifest.ByteSize, SessionID: started.Session.ID, State: uploadState, ConfirmedParts: started.ConfirmedParts, UpdatedAt: time.Now().UTC()}); err != nil {
			return &localsync.PublishError{Code: "LOCAL_UPLOAD_STATE_UNAVAILABLE", Retryable: true}
		}
	}
	hash := sha256.New()
	partCount := 0
	if manifest.ByteSize > 0 {
		partCount = int((manifest.ByteSize + int64(workspacedomain.WorkspaceUploadChunkSize) - 1) / int64(workspacedomain.WorkspaceUploadChunkSize))
	}
	if started.Completed {
		for partNo := 0; partNo < partCount; partNo++ {
			confirmed[partNo] = true
		}
	}
	for partNo := 0; partNo < partCount; partNo++ {
		partSize := int64(workspacedomain.WorkspaceUploadChunkSize)
		if partNo == partCount-1 {
			partSize = manifest.ByteSize - int64(partNo)*int64(workspacedomain.WorkspaceUploadChunkSize)
		}
		data := make([]byte, int(partSize))
		if _, err := io.ReadFull(file, data); err != nil {
			return &localsync.PublishError{Code: "WORKSPACE_FILE_CHANGED", Conflict: true}
		}
		_, _ = hash.Write(data)
		partDigest := sha256Digest(data)
		if !confirmed[partNo] {
			if err := p.client.Dispatch(ctx, "desktop.upload.part", application.UploadWorkspacePartInput{SessionID: started.Session.ID, PartNo: partNo, Digest: partDigest, Data: data}, &application.WorkspaceUploadPartResult{}); err != nil {
				return classifyDesktopPublishError(err)
			}
			confirmed[partNo] = true
		}
		if p.store != nil {
			if err := p.store.SaveUploadTransfer(ctx, localsync.UploadTransfer{ProjectID: command.ProjectID, Ref: manifest.Ref, ContentDigest: manifest.Digest, ByteSize: manifest.ByteSize, SessionID: started.Session.ID, State: uploadState, ConfirmedParts: confirmedPartNumbersFromMap(confirmed), UpdatedAt: time.Now().UTC()}); err != nil {
				return &localsync.PublishError{Code: "LOCAL_UPLOAD_STATE_UNAVAILABLE", Retryable: true}
			}
		}
	}
	var extra [1]byte
	if n, readErr := file.Read(extra[:]); readErr != io.EOF || n != 0 {
		return &localsync.PublishError{Code: "WORKSPACE_FILE_CHANGED", Conflict: true}
	}
	after, err := file.Stat()
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || sha256DigestFromHash(hash) != manifest.Digest {
		return &localsync.PublishError{Code: "WORKSPACE_FILE_CHANGED", Conflict: true}
	}
	if !started.Completed {
		if err := p.client.Dispatch(ctx, "desktop.upload.finalize", application.FinalizeWorkspaceUploadInput{SessionID: started.Session.ID}, &workspacedomain.WorkspaceObject{}); err != nil {
			return classifyDesktopPublishError(err)
		}
	}
	if p.store != nil {
		if err := p.store.SaveUploadTransfer(ctx, localsync.UploadTransfer{ProjectID: command.ProjectID, Ref: manifest.Ref, ContentDigest: manifest.Digest, ByteSize: manifest.ByteSize, SessionID: started.Session.ID, State: "completed", ConfirmedParts: confirmedPartNumbersFromMap(confirmed), UpdatedAt: time.Now().UTC()}); err != nil {
			return &localsync.PublishError{Code: "LOCAL_UPLOAD_STATE_UNAVAILABLE", Retryable: true}
		}
	}
	return nil
}

func (p desktopCloudPublisher) WorkspaceEvents(ctx context.Context, workspaceID, projectID string, after int64) (localsync.CloudEvents, error) {
	var page application.WorkspaceRevisionEvents
	if err := p.client.Dispatch(ctx, "desktop.workspace.events", map[string]any{"workspace_id": workspaceID, "project_id": projectID, "after": after, "limit": 100}, &page); err != nil {
		return localsync.CloudEvents{}, classifyDesktopPublishError(err)
	}
	events := make([]localsync.CloudRevisionEvent, 0, len(page.Events))
	for _, event := range page.Events {
		events = append(events, localsync.CloudRevisionEvent{ID: event.ID, WorkspaceID: event.WorkspaceID, ProjectID: event.ProjectID, RevisionNo: event.RevisionNo, ContentDigest: event.ContentDigest})
	}
	return localsync.CloudEvents{Events: events, NextCursor: page.NextCursor, ResyncRequired: page.ResyncRequired}, nil
}

func safeWorkspaceFilePath(root, ref string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(absoluteRoot, filepath.FromSlash(ref))
	relative, err := filepath.Rel(absoluteRoot, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("workspace file ref escapes root")
	}
	return candidate, nil
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sha256DigestFromHash(hash hash.Hash) string {
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func confirmedPartNumbersFromMap(parts map[int]bool) []int {
	result := make([]int, 0, len(parts))
	for partNo, confirmed := range parts {
		if confirmed {
			result = append(result, partNo)
		}
	}
	sort.Ints(result)
	return result
}

func workspaceRoots(workspaces []localconfig.DaemonWorkspace) map[string]string {
	result := make(map[string]string, len(workspaces))
	for _, workspace := range workspaces {
		if projectID, root := strings.TrimSpace(workspace.ProjectID), strings.TrimSpace(workspace.Root); projectID != "" && root != "" {
			result[projectID] = root
		}
	}
	return result
}

func classifyDesktopPublishError(err error) error {
	var domainErr *fault.Error
	if errors.As(err, &domainErr) {
		conflict := domainErr.Code == "WORKSPACE_REVISION_STALE" || strings.Contains(domainErr.Code, "IDEMPOTENCY") || domainErr.Code == "WORKSPACE_REVISION_UNCHANGED"
		auth := domainErr.Code == "DEVICE_TOKEN_INVALID" || domainErr.Code == "DEVICE_PROJECT_ACCESS_DENIED" || domainErr.Code == "WORKSPACE_BINDING_INVALID"
		return &localsync.PublishError{Code: domainErr.Code, Retryable: domainErr.Retryable && !auth && !conflict, Conflict: conflict}
	}
	return &localsync.PublishError{Code: "NETWORK_ERROR", Retryable: true}
}

func runDesktopSyncLoop(ctx context.Context, store *localsync.Store, runtimes []daemonBindingRuntime, logf func(string, ...any)) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	process := func() {
		for _, runtime := range runtimes {
			projects := make([]string, 0, len(runtime.binding.Workspaces))
			for _, workspace := range runtime.binding.Workspaces {
				if projectID := strings.TrimSpace(workspace.ProjectID); projectID != "" {
					projects = append(projects, projectID)
				}
				if strings.TrimSpace(workspace.Root) == "" {
					continue
				}
				observation, err := localsync.ObserveWorkspace(workspace.Root)
				if err != nil {
					logf("desktop workspace observation %s: %v", workspace.ProjectID, err)
					continue
				}
				if observation.ProjectID != workspace.ProjectID || observation.WorkspaceID != workspace.WorkspaceID {
					logf("desktop workspace observation %s: binding mismatch", workspace.ProjectID)
					continue
				}
				if _, err := store.ObserveProject(ctx, workspace.ProjectID, workspace.WorkspaceID, observation.Digest, time.Now().UTC()); err != nil {
					logf("desktop workspace projection %s: %v", workspace.ProjectID, err)
				}
			}
			processor := localsync.Processor{
				Store: store, Publisher: desktopCloudPublisher{client: runtime.client, store: store, workspaceRoots: workspaceRoots(runtime.binding.Workspaces)},
				WorkerID: "desktop-sync-" + runtime.binding.DeviceID, DeviceID: runtime.binding.DeviceID,
				ProjectIDs: projects,
			}
			if _, err := processor.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logf("desktop sync worker %s: %v", runtime.binding.DeviceID, err)
			}
			for _, workspace := range runtime.binding.Workspaces {
				if _, err := processor.ReconcileWorkspace(ctx, workspace.WorkspaceID, workspace.ProjectID); err != nil && !errors.Is(err, context.Canceled) {
					logf("desktop cloud reconciliation %s: %v", workspace.ProjectID, err)
				}
			}
		}
	}
	process()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			process()
		}
	}
}
