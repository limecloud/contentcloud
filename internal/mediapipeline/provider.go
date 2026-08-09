package mediapipeline

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
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
	RetryAfterSeconds int
	HTTPStatus        int
}

type Status struct {
	State             string
	Progress          int
	OutputRef         string
	ActualMinor       int64
	RetryAfterSeconds int
	HTTPStatus        int
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

//go:embed testdata/fake-provider-video.mp4
var fakeProviderVideo []byte

func (FakeProvider) Validate(request Request, profile domain.ProviderProfile) error {
	if request.JobID == "" || request.IdempotencyKey == "" || request.StoryboardSnapshotID == "" || request.DurationSeconds < 1 {
		return domain.Invalid("FAKE_PROVIDER_REQUEST_INVALID", "模拟服务商请求缺少任务、分镜或时长")
	}
	if !contains(profile.Modes, request.Mode) {
		return domain.Invalid("PROVIDER_MODE_UNSUPPORTED", "服务商配置不支持当前生成模式")
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
		return Status{}, domain.Invalid("FAKE_PROVIDER_JOB_INVALID", "模拟服务商返回的外部任务 ID 无效")
	}
	return Status{State: "succeeded", Progress: 100, OutputRef: "fake-output:" + strings.TrimPrefix(externalJobID, "fake-job-")}, nil
}

func (FakeProvider) Cancel(context.Context, string, domain.ProviderProfile) error { return nil }

func (FakeProvider) Download(_ context.Context, outputRef string, _ domain.ProviderProfile) (Download, error) {
	if !strings.HasPrefix(outputRef, "fake-output:") {
		return Download{}, domain.Invalid("FAKE_PROVIDER_OUTPUT_INVALID", "模拟服务商返回的输出引用无效")
	}
	return Download{Body: fakeMP4(outputRef), MediaType: "video/mp4", FileName: "generated-take.mp4"}, nil
}

func ValidateDownload(value Download, maxBytes int64) (map[string]any, error) {
	if len(value.Body) == 0 || int64(len(value.Body)) > maxBytes {
		return nil, domain.Invalid("MEDIA_OUTPUT_SIZE_INVALID", "服务商输出为空或超过大小限制")
	}
	if value.MediaType != "video/mp4" || len(value.Body) < 32 || string(value.Body[4:8]) != "ftyp" {
		return nil, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "服务商输出不是受支持的 MP4 容器")
	}
	boxes := map[string]bool{}
	for offset := 0; offset+8 <= len(value.Body); {
		size32 := binary.BigEndian.Uint32(value.Body[offset : offset+4])
		headerSize := uint64(8)
		var size uint64
		switch size32 {
		case 0:
			size = uint64(len(value.Body) - offset)
		case 1:
			if offset+16 > len(value.Body) {
				return nil, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 扩展数据块头不完整")
			}
			headerSize = 16
			size = binary.BigEndian.Uint64(value.Body[offset+8 : offset+16])
		default:
			size = uint64(size32)
		}
		if size < headerSize || size > uint64(len(value.Body)-offset) {
			return nil, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 数据块长度无效")
		}
		boxes[string(value.Body[offset+4:offset+8])] = true
		offset += int(size)
	}
	if !boxes["ftyp"] || !boxes["moov"] || !boxes["mdat"] {
		return nil, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 缺少必要的容器数据块")
	}
	return map[string]any{"container": "mp4", "codec": "fixture", "video_track": true, "audio_track": false, "validated": true}, nil
}

type StreamValidation struct {
	Technical map[string]any
	SHA256    string
	ByteSize  int64
}

// ValidateDownloadReader validates the MP4 box structure while consuming the
// response once. The same pass computes the content digest and enforces the
// byte ceiling, so a provider cannot make the worker buffer an unbounded body.
func ValidateDownloadReader(reader io.Reader, mediaType string, maxBytes int64) (StreamValidation, error) {
	if reader == nil || maxBytes <= 0 || strings.ToLower(strings.TrimSpace(mediaType)) != "video/mp4" {
		return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_INVALID", "服务商输出流为空、媒体类型不受支持或缺少大小上限")
	}
	hasher := &hashReader{reader: io.LimitReader(reader, maxBytes+1), hash: sha256.New()}
	var boxes = map[string]bool{}
	var total int64
	var header [8]byte
	for {
		n, err := io.ReadFull(hasher, header[:])
		if err == io.EOF && n == 0 {
			break
		}
		if err != nil {
			return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 数据块头不完整")
		}
		total = hasher.count
		if total > maxBytes {
			return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_SIZE_INVALID", "服务商输出为空或超过大小限制")
		}
		size32 := binary.BigEndian.Uint32(header[:4])
		boxType := string(header[4:8])
		if len(boxes) == 0 && boxType != "ftyp" {
			return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 首个数据块不是 ftyp")
		}
		boxes[boxType] = true
		headerSize := int64(8)
		var boxSize int64
		switch size32 {
		case 0:
			if _, err := io.Copy(io.Discard, hasher); err != nil {
				return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 数据块读取失败")
			}
			total = hasher.count
			break
		case 1:
			var extended [8]byte
			if _, err := io.ReadFull(hasher, extended[:]); err != nil {
				return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 扩展数据块头不完整")
			}
			total = hasher.count
			headerSize = 16
			boxSize64 := binary.BigEndian.Uint64(extended[:])
			if boxSize64 > uint64(^uint64(0)>>1) {
				return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 数据块长度无效")
			}
			boxSize = int64(boxSize64)
		default:
			boxSize = int64(size32)
		}
		if size32 != 0 {
			if boxSize < headerSize {
				return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 数据块长度无效")
			}
			payload := boxSize - headerSize
			if payload > maxBytes-total {
				return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_SIZE_INVALID", "服务商输出为空或超过大小限制")
			}
			copied, copyErr := io.CopyN(io.Discard, hasher, payload)
			if copyErr != nil || copied != payload {
				return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 数据块长度与响应不一致")
			}
			total = hasher.count
		}
		if size32 == 0 {
			break
		}
	}
	if total == 0 || total > maxBytes || !boxes["ftyp"] || !boxes["moov"] || !boxes["mdat"] {
		return StreamValidation{}, domain.Invalid("MEDIA_OUTPUT_CONTAINER_INVALID", "MP4 缺少必要的容器数据块")
	}
	return StreamValidation{Technical: map[string]any{"container": "mp4", "codec": "provider-stream", "video_track": true, "audio_track": false, "validated": true}, SHA256: hex.EncodeToString(hasher.hash.Sum(nil)), ByteSize: total}, nil
}

type hashReader struct {
	reader io.Reader
	hash   hash.Hash
	count  int64
}

func (r *hashReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.count += int64(n)
		if _, hashErr := r.hash.Write(p[:n]); hashErr != nil {
			return n, fmt.Errorf("计算媒体摘要失败: %w", hashErr)
		}
	}
	return n, err
}

func SHA256(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

func fakeMP4(string) []byte {
	return append([]byte(nil), fakeProviderVideo...)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
