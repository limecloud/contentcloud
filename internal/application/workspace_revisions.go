package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type PublishWorkspaceRevisionInput struct {
	WorkspaceID      string                                  `json:"workspace_id"`
	ProjectID        string                                  `json:"project_id"`
	BaseRevision     string                                  `json:"base_revision"`
	ContentDigest    string                                  `json:"content_digest"`
	Files            []workspacedomain.WorkspaceRevisionFile `json:"files"`
	ClientMutationID string                                  `json:"client_mutation_id"`
	IdempotencyKey   string                                  `json:"idempotency_key"`
}

type StartWorkspaceUploadInput struct {
	WorkspaceID    string `json:"workspace_id"`
	ProjectID      string `json:"project_id"`
	Ref            string `json:"ref"`
	ContentDigest  string `json:"content_digest"`
	ByteSize       int64  `json:"byte_size"`
	IdempotencyKey string `json:"idempotency_key"`
}

type WorkspaceUploadStartResult struct {
	Session        workspacedomain.WorkspaceUploadSession `json:"session"`
	ConfirmedParts []int                                  `json:"confirmed_parts"`
	Completed      bool                                   `json:"completed"`
	ContentDigest  string                                 `json:"content_digest"`
}

type UploadWorkspacePartInput struct {
	SessionID string `json:"session_id"`
	PartNo    int    `json:"part_no"`
	Digest    string `json:"digest"`
	Data      []byte `json:"data"`
}

type WorkspaceUploadPartResult struct {
	SessionID string `json:"session_id"`
	PartNo    int    `json:"part_no"`
	Digest    string `json:"digest"`
	ByteSize  int64  `json:"byte_size"`
}

type FinalizeWorkspaceUploadInput struct {
	SessionID string `json:"session_id"`
}

type WorkspaceRevisionEvents struct {
	SchemaVersion  string                              `json:"schema_version"`
	WorkspaceID    string                              `json:"workspace_id"`
	ProjectID      string                              `json:"project_id"`
	Events         []workspacedomain.WorkspaceRevision `json:"events"`
	NextCursor     int64                               `json:"next_cursor"`
	Gap            bool                                `json:"gap"`
	ResyncRequired bool                                `json:"resync_required"`
}

func (s *WorkspaceService) PublishWorkspaceRevision(ctx context.Context, actor Actor, input PublishWorkspaceRevisionInput) (workspacedomain.WorkspaceRevision, error) {
	if actor.Type != "device" || actor.DeviceID == "" {
		return workspacedomain.WorkspaceRevision{}, fault.Policy("DEVICE_ACTOR_REQUIRED", "发布工作区 Revision 需要设备身份", "重新授权本地 Daemon")
	}
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.BaseRevision = strings.TrimSpace(input.BaseRevision)
	input.ContentDigest = strings.TrimSpace(input.ContentDigest)
	input.ClientMutationID = strings.TrimSpace(input.ClientMutationID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.WorkspaceID == "" || input.ProjectID == "" || input.BaseRevision == "" || input.ClientMutationID == "" || input.IdempotencyKey == "" {
		return workspacedomain.WorkspaceRevision{}, fault.Invalid("WORKSPACE_REVISION_INPUT_INVALID", "工作区 Revision 缺少必填字段")
	}
	if !validWorkspaceRevisionDigest(input.ContentDigest) {
		return workspacedomain.WorkspaceRevision{}, fault.Invalid("WORKSPACE_REVISION_DIGEST_INVALID", "工作区 Revision 需要 SHA-256 摘要")
	}
	if err := validateWorkspaceRevisionFiles(input.Files, input.ContentDigest); err != nil {
		return workspacedomain.WorkspaceRevision{}, err
	}
	if !containsString(actor.ProjectIDs, input.ProjectID) {
		return workspacedomain.WorkspaceRevision{}, fault.Policy("DEVICE_PROJECT_ACCESS_DENIED", "设备未绑定该项目", "在项目设备设置中重新授权")
	}
	binding, err := s.workspace.WorkspaceBinding(ctx, actor.TenantID, input.WorkspaceID)
	if err != nil {
		return workspacedomain.WorkspaceRevision{}, err
	}
	if binding.ProjectID != input.ProjectID || binding.DeviceID != actor.DeviceID || binding.Status != "active" || binding.RevokedAt != nil {
		return workspacedomain.WorkspaceRevision{}, fault.Conflict("WORKSPACE_BINDING_INVALID", "工作区绑定无效或与当前设备不匹配")
	}
	digests := make([]string, 0, len(input.Files))
	for _, file := range input.Files {
		digests = append(digests, file.Digest)
	}
	objects, err := s.workspace.WorkspaceObjects(ctx, actor.TenantID, input.ProjectID, digests)
	if err != nil {
		return workspacedomain.WorkspaceRevision{}, err
	}
	objectByDigest := make(map[string]workspacedomain.WorkspaceObject, len(objects))
	for _, object := range objects {
		objectByDigest[object.ContentDigest] = object
	}
	for _, file := range input.Files {
		object, ok := objectByDigest[file.Digest]
		if !ok || object.ByteSize != file.ByteSize {
			return workspacedomain.WorkspaceRevision{}, fault.Conflict("WORKSPACE_FILE_OBJECT_MISSING", "Revision 引用了尚未完成上传的文件")
		}
	}
	return s.workspace.PublishWorkspaceRevision(ctx, workspacedomain.WorkspaceRevision{
		ID: idgen.New(), TenantID: actor.TenantID, ProjectID: input.ProjectID, WorkspaceID: input.WorkspaceID, DeviceID: actor.DeviceID,
		SchemaVersion: workspacedomain.WorkspaceRevisionSchemaVersion, BaseRevisionID: input.BaseRevision, ContentDigest: input.ContentDigest, Files: input.Files,
		ClientMutationID: input.ClientMutationID, IdempotencyKey: input.IdempotencyKey, CreatedAt: s.now().UTC(),
	})
}

func (s *WorkspaceService) StartWorkspaceUpload(ctx context.Context, actor Actor, input StartWorkspaceUploadInput) (WorkspaceUploadStartResult, error) {
	input.WorkspaceID, input.ProjectID, input.Ref = strings.TrimSpace(input.WorkspaceID), strings.TrimSpace(input.ProjectID), strings.TrimSpace(input.Ref)
	input.ContentDigest, input.IdempotencyKey = strings.TrimSpace(input.ContentDigest), strings.TrimSpace(input.IdempotencyKey)
	if input.WorkspaceID == "" || input.ProjectID == "" || input.Ref == "" || input.IdempotencyKey == "" || input.ByteSize < 0 || input.ByteSize > workspacedomain.WorkspaceUploadMaxFileSize {
		return WorkspaceUploadStartResult{}, fault.Invalid("WORKSPACE_UPLOAD_INPUT_INVALID", "工作区上传缺少必填字段或文件大小无效")
	}
	if !workspacedomain.ValidWorkspaceRevisionFileRef(input.Ref) {
		return WorkspaceUploadStartResult{}, fault.Invalid("WORKSPACE_UPLOAD_REF_INVALID", "工作区文件路径不在受保护的内容目录中")
	}
	if !validWorkspaceRevisionDigest(input.ContentDigest) {
		return WorkspaceUploadStartResult{}, fault.Invalid("WORKSPACE_UPLOAD_DIGEST_INVALID", "工作区上传需要 SHA-256 摘要")
	}
	if _, err := s.requireDeviceWorkspace(ctx, actor, input.WorkspaceID, input.ProjectID); err != nil {
		return WorkspaceUploadStartResult{}, err
	}
	if object, err := s.workspace.WorkspaceObject(ctx, actor.TenantID, input.ProjectID, input.ContentDigest); err == nil {
		if object.ByteSize != input.ByteSize {
			return WorkspaceUploadStartResult{}, fault.Conflict("WORKSPACE_OBJECT_CONFLICT", "相同摘要对应了不同的文件大小")
		}
		return WorkspaceUploadStartResult{Completed: true, ContentDigest: object.ContentDigest, ConfirmedParts: confirmedPartNumbers(nil)}, nil
	} else if !fault.IsNotFound(err) {
		return WorkspaceUploadStartResult{}, err
	}
	partCount := 0
	if input.ByteSize > 0 {
		partCount = int((input.ByteSize + workspacedomain.WorkspaceUploadChunkSize - 1) / workspacedomain.WorkspaceUploadChunkSize)
	}
	now := s.now().UTC()
	session, err := s.workspace.CreateWorkspaceUploadSession(ctx, workspacedomain.WorkspaceUploadSession{
		ID: idgen.New(), TenantID: actor.TenantID, ProjectID: input.ProjectID, WorkspaceID: input.WorkspaceID, DeviceID: actor.DeviceID,
		Ref: input.Ref, ContentDigest: input.ContentDigest, ByteSize: input.ByteSize, ChunkSize: workspacedomain.WorkspaceUploadChunkSize,
		PartCount: partCount, State: "initiated", ObjectKey: workspaceObjectKey(actor.TenantID, input.ProjectID, input.ContentDigest),
		IdempotencyKey: input.IdempotencyKey, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil {
		return WorkspaceUploadStartResult{}, err
	}
	parts, err := s.workspace.WorkspaceUploadParts(ctx, actor.TenantID, session.ID)
	if err != nil {
		return WorkspaceUploadStartResult{}, err
	}
	return WorkspaceUploadStartResult{Session: session, ConfirmedParts: confirmedPartNumbers(parts), Completed: session.State == "completed", ContentDigest: session.ContentDigest}, nil
}

func (s *WorkspaceService) UploadWorkspacePart(ctx context.Context, actor Actor, input UploadWorkspacePartInput) (WorkspaceUploadPartResult, error) {
	input.SessionID, input.Digest = strings.TrimSpace(input.SessionID), strings.TrimSpace(input.Digest)
	session, err := s.workspace.WorkspaceUploadSession(ctx, actor.TenantID, input.SessionID)
	if err != nil {
		return WorkspaceUploadPartResult{}, err
	}
	if _, err := s.requireDeviceWorkspace(ctx, actor, session.WorkspaceID, session.ProjectID); err != nil {
		return WorkspaceUploadPartResult{}, err
	}
	if session.State == "completed" {
		return WorkspaceUploadPartResult{}, fault.Conflict("WORKSPACE_UPLOAD_COMPLETED", "上传会话已经完成")
	}
	if !s.now().UTC().Before(session.ExpiresAt) {
		return WorkspaceUploadPartResult{}, fault.Conflict("WORKSPACE_UPLOAD_EXPIRED", "工作区上传会话已过期")
	}
	if input.PartNo < 0 || input.PartNo >= session.PartCount {
		return WorkspaceUploadPartResult{}, fault.Invalid("WORKSPACE_UPLOAD_PART_INVALID", "分片编号超出上传会话范围")
	}
	expectedSize := int64(workspacedomain.WorkspaceUploadChunkSize)
	if input.PartNo == session.PartCount-1 {
		expectedSize = session.ByteSize - int64(input.PartNo)*session.ChunkSize
	}
	if int64(len(input.Data)) != expectedSize {
		return WorkspaceUploadPartResult{}, fault.Invalid("WORKSPACE_UPLOAD_PART_SIZE_INVALID", "分片大小与上传会话不匹配")
	}
	if !validWorkspaceRevisionDigest(input.Digest) {
		return WorkspaceUploadPartResult{}, fault.Invalid("WORKSPACE_UPLOAD_PART_DIGEST_INVALID", "分片需要 SHA-256 摘要")
	}
	sum := sha256.Sum256(input.Data)
	if "sha256:"+hex.EncodeToString(sum[:]) != input.Digest {
		return WorkspaceUploadPartResult{}, fault.Conflict("WORKSPACE_UPLOAD_PART_DIGEST_MISMATCH", "分片摘要校验失败")
	}
	objectKey := workspaceUploadPartObjectKey(session, input.PartNo, input.Digest)
	if err := s.blobs.Put(ctx, objectKey, input.Data); err != nil {
		return WorkspaceUploadPartResult{}, err
	}
	part, err := s.workspace.SaveWorkspaceUploadPart(ctx, actor.TenantID, workspacedomain.WorkspaceUploadPart{SessionID: session.ID, PartNo: input.PartNo, Digest: input.Digest, ByteSize: int64(len(input.Data)), ObjectKey: objectKey, CreatedAt: s.now().UTC()})
	if err != nil {
		if deleter, ok := s.blobs.(blob.DeleteStore); ok {
			_ = deleter.Delete(ctx, objectKey)
		}
		return WorkspaceUploadPartResult{}, err
	}
	return WorkspaceUploadPartResult{SessionID: part.SessionID, PartNo: part.PartNo, Digest: part.Digest, ByteSize: part.ByteSize}, nil
}

func (s *WorkspaceService) FinalizeWorkspaceUpload(ctx context.Context, actor Actor, input FinalizeWorkspaceUploadInput) (workspacedomain.WorkspaceObject, error) {
	input.SessionID = strings.TrimSpace(input.SessionID)
	session, err := s.workspace.WorkspaceUploadSession(ctx, actor.TenantID, input.SessionID)
	if err != nil {
		return workspacedomain.WorkspaceObject{}, err
	}
	if _, err := s.requireDeviceWorkspace(ctx, actor, session.WorkspaceID, session.ProjectID); err != nil {
		return workspacedomain.WorkspaceObject{}, err
	}
	if session.State == "completed" {
		return s.workspace.WorkspaceObject(ctx, actor.TenantID, session.ProjectID, session.ContentDigest)
	}
	if !s.now().UTC().Before(session.ExpiresAt) {
		return workspacedomain.WorkspaceObject{}, fault.Conflict("WORKSPACE_UPLOAD_EXPIRED", "工作区上传会话已过期")
	}
	parts, err := s.workspace.WorkspaceUploadParts(ctx, actor.TenantID, session.ID)
	if err != nil {
		return workspacedomain.WorkspaceObject{}, err
	}
	if len(parts) != session.PartCount {
		return workspacedomain.WorkspaceObject{}, fault.Conflict("WORKSPACE_UPLOAD_INCOMPLETE", "工作区上传仍缺少分片")
	}
	partByNo := make(map[int]workspacedomain.WorkspaceUploadPart, len(parts))
	for _, part := range parts {
		if _, exists := partByNo[part.PartNo]; exists {
			return workspacedomain.WorkspaceObject{}, fault.Conflict("WORKSPACE_UPLOAD_PART_CONFLICT", "上传会话包含重复分片")
		}
		partByNo[part.PartNo] = part
	}
	temporary, err := os.CreateTemp("", "contentcloud-workspace-upload-*")
	if err != nil {
		return workspacedomain.WorkspaceObject{}, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	defer temporary.Close()
	hash := sha256.New()
	var total int64
	for partNo := 0; partNo < session.PartCount; partNo++ {
		part, ok := partByNo[partNo]
		if !ok {
			return workspacedomain.WorkspaceObject{}, fault.Conflict("WORKSPACE_UPLOAD_INCOMPLETE", "工作区上传仍缺少分片")
		}
		data, err := s.blobs.Get(ctx, part.ObjectKey)
		if err != nil {
			return workspacedomain.WorkspaceObject{}, fault.Conflict("WORKSPACE_UPLOAD_PART_MISSING", "上传分片对象不可读取")
		}
		if int64(len(data)) != part.ByteSize {
			return workspacedomain.WorkspaceObject{}, fault.Conflict("WORKSPACE_UPLOAD_PART_SIZE_INVALID", "上传分片对象大小已变化")
		}
		sum := sha256.Sum256(data)
		if "sha256:"+hex.EncodeToString(sum[:]) != part.Digest {
			return workspacedomain.WorkspaceObject{}, fault.Conflict("WORKSPACE_UPLOAD_PART_DIGEST_MISMATCH", "上传分片对象摘要已变化")
		}
		if _, err := io.Copy(io.MultiWriter(temporary, hash), bytes.NewReader(data)); err != nil {
			return workspacedomain.WorkspaceObject{}, err
		}
		total += int64(len(data))
	}
	if total != session.ByteSize {
		return workspacedomain.WorkspaceObject{}, fault.Conflict("WORKSPACE_UPLOAD_SIZE_MISMATCH", "合并后的文件大小与声明不一致")
	}
	if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != session.ContentDigest {
		return workspacedomain.WorkspaceObject{}, fault.Conflict("WORKSPACE_UPLOAD_DIGEST_MISMATCH", "合并后的文件摘要与声明不一致")
	}
	if err := temporary.Sync(); err != nil {
		return workspacedomain.WorkspaceObject{}, err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return workspacedomain.WorkspaceObject{}, err
	}
	readerStore, ok := s.blobs.(blob.ReaderStore)
	if !ok {
		return workspacedomain.WorkspaceObject{}, fault.Policy("WORKSPACE_BLOB_READER_REQUIRED", "当前对象存储不支持大文件上传", "配置支持流式写入的本地或 S3 存储")
	}
	now := s.now().UTC()
	object := workspacedomain.WorkspaceObject{TenantID: session.TenantID, ProjectID: session.ProjectID, ContentDigest: session.ContentDigest, ByteSize: session.ByteSize, ObjectKey: session.ObjectKey, CreatedAt: now}
	if err := readerStore.PutReader(ctx, object.ObjectKey, temporary, session.ByteSize); err != nil {
		return workspacedomain.WorkspaceObject{}, err
	}
	session.UpdatedAt = now
	if err := s.workspace.CompleteWorkspaceUpload(ctx, session, object); err != nil {
		if deleter, ok := s.blobs.(blob.DeleteStore); ok {
			_ = deleter.Delete(ctx, object.ObjectKey)
		}
		return workspacedomain.WorkspaceObject{}, err
	}
	if deleter, ok := s.blobs.(blob.DeleteStore); ok {
		for _, part := range parts {
			_ = deleter.Delete(ctx, part.ObjectKey)
		}
	}
	return object, nil
}

func (s *WorkspaceService) LatestWorkspaceRevision(ctx context.Context, actor Actor, workspaceID, projectID string) (workspacedomain.WorkspaceRevision, error) {
	if _, err := s.requireDeviceWorkspace(ctx, actor, workspaceID, projectID); err != nil {
		return workspacedomain.WorkspaceRevision{}, err
	}
	return s.workspace.LatestWorkspaceRevision(ctx, actor.TenantID, workspaceID)
}

func (s *WorkspaceService) WorkspaceRevisionEvents(ctx context.Context, actor Actor, workspaceID, projectID string, after int64, limit int) (WorkspaceRevisionEvents, error) {
	if after < 0 || limit < 1 || limit > 200 {
		return WorkspaceRevisionEvents{}, fault.Invalid("WORKSPACE_REVISION_CURSOR_INVALID", "工作区 Revision 游标或数量无效")
	}
	if _, err := s.requireDeviceWorkspace(ctx, actor, workspaceID, projectID); err != nil {
		return WorkspaceRevisionEvents{}, err
	}
	latest, latestErr := s.workspace.LatestWorkspaceRevision(ctx, actor.TenantID, workspaceID)
	if latestErr != nil && !fault.IsNotFound(latestErr) {
		return WorkspaceRevisionEvents{}, latestErr
	}
	if fault.IsNotFound(latestErr) {
		latest = workspacedomain.WorkspaceRevision{}
	}
	if after > latest.RevisionNo {
		return WorkspaceRevisionEvents{SchemaVersion: "contentcloud.workspace-revision-events/1.0", WorkspaceID: workspaceID, ProjectID: projectID, Events: []workspacedomain.WorkspaceRevision{}, NextCursor: latest.RevisionNo, Gap: true, ResyncRequired: true}, nil
	}
	events, err := s.workspace.WorkspaceRevisionsAfter(ctx, actor.TenantID, workspaceID, after, limit)
	if err != nil {
		return WorkspaceRevisionEvents{}, err
	}
	next := after
	if len(events) > 0 {
		next = events[len(events)-1].RevisionNo
	}
	gap := len(events) > 0 && events[0].RevisionNo != after+1
	return WorkspaceRevisionEvents{SchemaVersion: "contentcloud.workspace-revision-events/1.0", WorkspaceID: workspaceID, ProjectID: projectID, Events: events, NextCursor: next, Gap: gap, ResyncRequired: gap}, nil
}

func (s *WorkspaceService) requireDeviceWorkspace(ctx context.Context, actor Actor, workspaceID, projectID string) (workspacedomain.WorkspaceBinding, error) {
	if actor.Type != "device" || !containsString(actor.ProjectIDs, projectID) {
		return workspacedomain.WorkspaceBinding{}, fault.Policy("DEVICE_PROJECT_ACCESS_DENIED", "设备未绑定该项目", "在项目设备设置中重新授权")
	}
	binding, err := s.workspace.WorkspaceBinding(ctx, actor.TenantID, workspaceID)
	if err != nil {
		return workspacedomain.WorkspaceBinding{}, err
	}
	if binding.ProjectID != projectID || binding.DeviceID != actor.DeviceID || binding.Status != "active" || binding.RevokedAt != nil {
		return workspacedomain.WorkspaceBinding{}, fault.Conflict("WORKSPACE_BINDING_INVALID", "工作区绑定无效或与当前设备不匹配")
	}
	return binding, nil
}

func validateWorkspaceRevisionFiles(files []workspacedomain.WorkspaceRevisionFile, contentDigest string) error {
	previous := ""
	for _, file := range files {
		if !workspacedomain.ValidWorkspaceRevisionFileRef(file.Ref) || (previous != "" && file.Ref <= previous) || file.ByteSize < 0 || file.ByteSize > workspacedomain.WorkspaceUploadMaxFileSize || !validWorkspaceRevisionDigest(file.Digest) {
			return fault.Invalid("WORKSPACE_REVISION_FILES_INVALID", "工作区 Revision 文件清单无效")
		}
		previous = file.Ref
	}
	if workspacedomain.WorkspaceContentDigest(files) != contentDigest {
		return fault.Conflict("WORKSPACE_REVISION_DIGEST_MISMATCH", "工作区 Revision 摘要与文件清单不一致")
	}
	return nil
}

func confirmedPartNumbers(parts []workspacedomain.WorkspaceUploadPart) []int {
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		result = append(result, part.PartNo)
	}
	slices.Sort(result)
	return result
}

func workspaceObjectKey(tenantID, projectID, digest string) string {
	return "workspace-objects/" + tenantID + "/" + projectID + "/" + strings.TrimPrefix(digest, "sha256:")
}

func workspaceUploadPartObjectKey(session workspacedomain.WorkspaceUploadSession, partNo int, digest string) string {
	return "workspace-upload-parts/" + session.TenantID + "/" + session.ID + "/" + path.Base(strings.TrimPrefix(digest, "sha256:")) + "-" + fmt.Sprint(partNo)
}

func validWorkspaceRevisionDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
