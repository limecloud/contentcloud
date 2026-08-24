// Package composition contains the execution port for deterministic media
// composition. It deliberately owns no task, review, artifact, or delivery
// state; the application layer supplies already-verified inputs and persists
// the returned bytes through the existing Artifact transaction.
package composition

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const (
	RendererName              = "contentcloud.deterministic-compositor"
	RendererVersion           = "1.0.0"
	maxCompositionBytes int64 = 100 << 20
)

// InputArtifact is a transient, digest-checked input to a composition run.
// It is not a second persistence model and is never exposed as a business
// fact by this package.
type InputArtifact struct {
	ID        string
	Digest    string
	MediaType string
	FileName  string
	Body      []byte
}

type Request struct {
	Manifest       deliverydomain.CompositionManifest
	ManifestDigest string
	SourceArtifact InputArtifact
	Auxiliary      []InputArtifact
}

type Result struct {
	Body      []byte
	MediaType string
	FileName  string
	Technical map[string]any
}

type Worker interface {
	Compose(context.Context, Request) (Result, error)
}

// DeterministicWorker creates an independent MP4 object by adding a
// deterministic ContentCloud composition box to the verified source
// container. The source media bytes are retained so this worker is safe in
// environments without a native encoder; narration, subtitle, timeline and
// brand/CTA inputs are still fixed in the output's auditable composition
// metadata. A native encoder can be injected through WithCompositionWorker
// without changing the application or domain contracts.
type DeterministicWorker struct{}

func NewDeterministicWorker() Worker { return DeterministicWorker{} }

func (DeterministicWorker) Compose(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := request.Manifest.Validate(); err != nil {
		return Result{}, err
	}
	if request.Manifest.Renderer.Name != RendererName || request.Manifest.Renderer.Version != RendererVersion {
		return Result{}, fault.Policy("COMPOSITION_RENDERER_MISMATCH", "合成 Worker 与 Manifest 声明的 renderer 不一致", "使用已批准的确定性合成 renderer")
	}
	if strings.TrimSpace(request.ManifestDigest) == "" {
		return Result{}, fault.Invalid("COMPOSITION_MANIFEST_DIGEST_REQUIRED", "合成 Worker 必须接收 manifest digest")
	}
	computedDigest, err := request.Manifest.Digest()
	if err != nil {
		return Result{}, err
	}
	if !strings.EqualFold(computedDigest, request.ManifestDigest) {
		return Result{}, fault.Conflict("COMPOSITION_MANIFEST_DIGEST_MISMATCH", "Worker 收到的 manifest digest 与内容不一致")
	}
	if err := validateInput(request.SourceArtifact, true); err != nil {
		return Result{}, err
	}
	if _, err := mediapipeline.ValidateDownload(mediapipeline.Download{Body: request.SourceArtifact.Body, MediaType: request.SourceArtifact.MediaType, FileName: request.SourceArtifact.FileName}, maxCompositionBytes); err != nil {
		return Result{}, err
	}
	auxiliary := append([]InputArtifact(nil), request.Auxiliary...)
	sort.Slice(auxiliary, func(i, j int) bool { return auxiliary[i].ID < auxiliary[j].ID })
	seen := map[string]struct{}{}
	for _, input := range auxiliary {
		if _, ok := seen[input.ID]; ok {
			return Result{}, fault.Invalid("COMPOSITION_INPUT_DUPLICATE", "合成 Worker 的辅助输入不能重复")
		}
		seen[input.ID] = struct{}{}
		if err := validateInput(input, false); err != nil {
			return Result{}, err
		}
	}
	metadata := compositionMetadata(request.Manifest, request.ManifestDigest, request.SourceArtifact, auxiliary)
	metadataBody, err := json.Marshal(metadata)
	if err != nil {
		return Result{}, fault.Invalid("COMPOSITION_METADATA_ENCODE_FAILED", "合成输入血缘无法编码")
	}
	result, err := appendCompositionBox(request.SourceArtifact.Body, metadataBody)
	if err != nil {
		return Result{}, err
	}
	if int64(len(result)) > maxCompositionBytes {
		return Result{}, fault.Invalid("COMPOSITION_OUTPUT_SIZE_INVALID", "确定性合成输出超过大小限制")
	}
	technical := map[string]any{
		"container":           "mp4",
		"codec":               "deterministic-compositor",
		"composition_worker":  RendererName,
		"composition_version": RendererVersion,
		"manifest_digest":     request.ManifestDigest,
		"input_count":         len(auxiliary) + 1,
		"validated":           true,
	}
	return Result{Body: result, MediaType: "video/mp4", FileName: "composed-" + filepath.Base(request.SourceArtifact.FileName), Technical: technical}, nil
}

func validateInput(input InputArtifact, video bool) error {
	if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.Digest) == "" || len(input.Body) == 0 {
		return fault.Invalid("COMPOSITION_INPUT_INVALID", "合成输入必须包含 ID、摘要和内容")
	}
	if !strings.EqualFold(normalizeDigest(mediapipeline.SHA256(input.Body)), normalizeDigest(input.Digest)) {
		return fault.Conflict("COMPOSITION_INPUT_DIGEST_MISMATCH", "合成输入内容与声明摘要不一致")
	}
	if strings.TrimSpace(input.MediaType) == "" {
		return fault.Invalid("COMPOSITION_INPUT_MEDIA_TYPE_INVALID", "合成输入缺少媒体类型")
	}
	if video && input.MediaType != "video/mp4" {
		return fault.Invalid("COMPOSITION_SOURCE_MEDIA_TYPE_INVALID", "合成源必须是 video/mp4")
	}
	return nil
}

func normalizeDigest(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "sha256:")
}

type compositionMetadataValue struct {
	Version        string                     `json:"version"`
	ManifestDigest string                     `json:"manifest_digest"`
	Manifest       json.RawMessage            `json:"manifest"`
	Source         compositionInputMetadata   `json:"source"`
	Auxiliary      []compositionInputMetadata `json:"auxiliary"`
}

type compositionInputMetadata struct {
	ID        string `json:"id"`
	Digest    string `json:"digest"`
	MediaType string `json:"media_type"`
	FileName  string `json:"file_name,omitempty"`
	ByteSize  int    `json:"byte_size"`
}

func compositionMetadata(manifest deliverydomain.CompositionManifest, digest string, source InputArtifact, auxiliary []InputArtifact) compositionMetadataValue {
	canonical, _ := manifest.CanonicalJSON()
	aux := make([]compositionInputMetadata, 0, len(auxiliary))
	for _, input := range auxiliary {
		aux = append(aux, compositionInputMetadata{ID: input.ID, Digest: input.Digest, MediaType: input.MediaType, FileName: input.FileName, ByteSize: len(input.Body)})
	}
	return compositionMetadataValue{Version: "contentcloud.composition-output/1.0", ManifestDigest: digest, Manifest: canonical, Source: compositionInputMetadata{ID: source.ID, Digest: source.Digest, MediaType: source.MediaType, FileName: source.FileName, ByteSize: len(source.Body)}, Auxiliary: aux}
}

func appendCompositionBox(source, metadata []byte) ([]byte, error) {
	payload := append([]byte("CCMP/1\x00"), metadata...)
	boxSize := int64(8 + len(payload))
	if boxSize > int64(^uint32(0)) {
		return nil, fault.Invalid("COMPOSITION_METADATA_TOO_LARGE", "合成血缘元数据超过 MP4 box 大小限制")
	}
	box := make([]byte, boxSize)
	binary.BigEndian.PutUint32(box[:4], uint32(boxSize))
	copy(box[4:8], []byte("free"))
	copy(box[8:], payload)
	result := make([]byte, 0, len(source)+len(box))
	result = append(result, source...)
	result = append(result, box...)
	return result, nil
}
