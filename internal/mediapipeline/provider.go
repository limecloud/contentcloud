package mediapipeline

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type Request struct {
	JobID                string
	IdempotencyKey       string
	StoryboardSnapshotID string
	Mode                 string
	AspectRatio          string
	DurationSeconds      int
	InputArtifactRefs    []string
}

type Estimate struct {
	CostMinor int64
	Currency  string
}

type Submission struct {
	ExternalJobID     string
	ProviderRequestID string
}

type Status struct {
	State       string
	Progress    int
	OutputRef   string
	ActualMinor int64
}

type Download struct {
	Body      []byte
	MediaType string
	FileName  string
}

type Adapter interface {
	Validate(Request, domain.ProviderProfile) error
	Estimate(Request, domain.ProviderProfile) (Estimate, error)
	Submit(context.Context, Request, domain.ProviderProfile) (Submission, error)
	Status(context.Context, string, domain.ProviderProfile) (Status, error)
	Cancel(context.Context, string, domain.ProviderProfile) error
	Download(context.Context, string, domain.ProviderProfile) (Download, error)
}

type FakeProvider struct{}

func (FakeProvider) Validate(request Request, profile domain.ProviderProfile) error {
	if request.JobID == "" || request.IdempotencyKey == "" || request.StoryboardSnapshotID == "" || request.DurationSeconds < 1 {
		return domain.Invalid("FAKE_PROVIDER_REQUEST_INVALID", "FakeProvider 请求缺少 Job、分镜或时长")
	}
	if !contains(profile.Modes, request.Mode) {
		return domain.Invalid("PROVIDER_MODE_UNSUPPORTED", "Provider Profile 不支持当前生成模式")
	}
	return nil
}

func (FakeProvider) Estimate(Request, domain.ProviderProfile) (Estimate, error) {
	return Estimate{CostMinor: 0, Currency: "CNY"}, nil
}

func (FakeProvider) Submit(_ context.Context, request Request, _ domain.ProviderProfile) (Submission, error) {
	hash := sha256.Sum256([]byte(request.IdempotencyKey))
	ref := hex.EncodeToString(hash[:8])
	return Submission{ExternalJobID: "fake-job-" + ref, ProviderRequestID: "fake-request-" + ref}, nil
}

func (FakeProvider) Status(_ context.Context, externalJobID string, _ domain.ProviderProfile) (Status, error) {
	if !strings.HasPrefix(externalJobID, "fake-job-") {
		return Status{}, domain.Invalid("FAKE_PROVIDER_JOB_INVALID", "FakeProvider external job id 无效")
	}
	return Status{State: "succeeded", Progress: 100, OutputRef: "fake-output:" + strings.TrimPrefix(externalJobID, "fake-job-")}, nil
}

func (FakeProvider) Cancel(context.Context, string, domain.ProviderProfile) error { return nil }

func (FakeProvider) Download(_ context.Context, outputRef string, _ domain.ProviderProfile) (Download, error) {
	if !strings.HasPrefix(outputRef, "fake-output:") {
		return Download{}, domain.Invalid("FAKE_PROVIDER_OUTPUT_INVALID", "FakeProvider output ref 无效")
	}
	return Download{Body: fakeMP4(outputRef), MediaType: "video/mp4", FileName: "generated-take.mp4"}, nil
}

func ValidateDownload(value Download, maxBytes int64) (map[string]any, error) {
	if len(value.Body) == 0 || int64(len(value.Body)) > maxBytes {
		return nil, domain.Invalid("MEDIA_OUTPUT_SIZE_INVALID", "Provider 输出为空或超过大小限制")
	}
	if value.MediaType != "video/mp4" || len(value.Body) < 32 || string(value.Body[4:8]) != "ftyp" {
		return nil, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "Provider 输出不是受支持的 MP4 容器")
	}
	boxes := map[string]bool{}
	for offset := 0; offset+8 <= len(value.Body); {
		size := int(binary.BigEndian.Uint32(value.Body[offset : offset+4]))
		if size < 8 || offset+size > len(value.Body) {
			return nil, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 box 长度无效")
		}
		boxes[string(value.Body[offset+4:offset+8])] = true
		offset += size
	}
	if !boxes["ftyp"] || !boxes["moov"] || !boxes["mdat"] {
		return nil, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 缺少必要容器 box")
	}
	return map[string]any{"container": "mp4", "codec": "fixture", "video_track": true, "audio_track": false, "validated": true}, nil
}

func SHA256(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

func fakeMP4(seed string) []byte {
	ftypPayload := []byte("isom\x00\x00\x02\x00isomiso2")
	moovPayload := []byte("contentcloud-fixture-video")
	mdatHash := sha256.Sum256([]byte(seed))
	result := []byte{}
	result = append(result, mp4Box("ftyp", ftypPayload)...)
	result = append(result, mp4Box("moov", moovPayload)...)
	result = append(result, mp4Box("mdat", mdatHash[:])...)
	return result
}

func mp4Box(kind string, payload []byte) []byte {
	if len(kind) != 4 {
		panic(fmt.Sprintf("invalid MP4 box kind %q", kind))
	}
	result := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(result[:4], uint32(len(result)))
	copy(result[4:8], kind)
	copy(result[8:], payload)
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
