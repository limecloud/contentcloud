package delivery

import (
	"encoding/json"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

const CompositionManifestSchema = "contentcloud.composition-manifest/1.0"

type CompositionManifest struct {
	Schema           CompositionSchemaRef   `json:"schema"`
	ApprovedSnapshot CompositionSnapshotRef `json:"approved_snapshot"`
	SelectedVideo    CompositionVideoRef    `json:"selected_video"`
	Narration        CompositionNarration   `json:"narration"`
	Subtitles        CompositionSubtitles   `json:"subtitles"`
	BrandCTA         CompositionBrandCTA    `json:"brand_cta"`
	Timeline         CompositionTimeline    `json:"timeline"`
	Renderer         CompositionRenderer    `json:"renderer"`
	Output           CompositionOutput      `json:"output"`
}

type CompositionSchemaRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type CompositionSnapshotRef struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}

type CompositionVideoRef struct {
	ArtifactID    string `json:"artifact_id"`
	Digest        string `json:"digest"`
	MediaReviewID string `json:"media_review_id"`
	ReviewDigest  string `json:"review_digest"`
}

type CompositionNarration struct {
	Status            string `json:"status"`
	ArtifactID        string `json:"artifact_id,omitempty"`
	Digest            string `json:"digest,omitempty"`
	Language          string `json:"language,omitempty"`
	VoiceConfigDigest string `json:"voice_config_digest,omitempty"`
}

type CompositionSubtitles struct {
	Status        string `json:"status"`
	SourceKind    string `json:"source_kind"`
	ArtifactID    string `json:"artifact_id,omitempty"`
	Digest        string `json:"digest,omitempty"`
	ContentDigest string `json:"content_digest,omitempty"`
	Language      string `json:"language,omitempty"`
	StyleVersion  string `json:"style_version,omitempty"`
}

type CompositionBrandCTA struct {
	BrandConfigDigest string `json:"brand_config_digest"`
	CTADigest         string `json:"cta_digest"`
}

type CompositionTimeline struct {
	CanvasWidth          int                  `json:"canvas_width"`
	CanvasHeight         int                  `json:"canvas_height"`
	FrameRateNumerator   int                  `json:"frame_rate_numerator"`
	FrameRateDenominator int                  `json:"frame_rate_denominator"`
	DurationMS           int64                `json:"duration_ms"`
	TransitionVersion    string               `json:"transition_version"`
	Segments             []CompositionSegment `json:"segments"`
}

type CompositionSegment struct {
	ID             string `json:"id"`
	StartMS        int64  `json:"start_ms"`
	EndMS          int64  `json:"end_ms"`
	ArtifactID     string `json:"artifact_id"`
	ArtifactDigest string `json:"artifact_digest"`
	TransitionIn   string `json:"transition_in,omitempty"`
	TransitionOut  string `json:"transition_out,omitempty"`
}

type CompositionRenderer struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	CapabilityDigest string `json:"capability_digest"`
}

type CompositionOutput struct {
	Kind       string `json:"kind"`
	MediaType  string `json:"media_type"`
	Container  string `json:"container"`
	VideoCodec string `json:"video_codec"`
	AudioCodec string `json:"audio_codec"`
}

func (v CompositionManifest) Validate() error {
	if v.Schema.Name != CompositionManifestSchema || strings.TrimSpace(v.Schema.Version) == "" {
		return fault.Invalid("COMPOSITION_MANIFEST_SCHEMA_INVALID", "合成清单必须使用受支持的 schema 名称和版本")
	}
	if strings.TrimSpace(v.ApprovedSnapshot.ID) == "" || !stablehash.Valid(v.ApprovedSnapshot.Digest) {
		return fault.Invalid("COMPOSITION_MANIFEST_SNAPSHOT_INVALID", "合成清单必须绑定有效的批准快照摘要")
	}
	if strings.TrimSpace(v.SelectedVideo.ArtifactID) == "" || strings.TrimSpace(v.SelectedVideo.MediaReviewID) == "" || !stablehash.Valid(v.SelectedVideo.Digest) || !stablehash.Valid(v.SelectedVideo.ReviewDigest) {
		return fault.Invalid("COMPOSITION_MANIFEST_VIDEO_INVALID", "合成清单必须绑定选中视频 Artifact、审核和摘要")
	}
	if err := v.Narration.validate(); err != nil {
		return err
	}
	if err := v.Subtitles.validate(); err != nil {
		return err
	}
	if !stablehash.Valid(v.BrandCTA.BrandConfigDigest) || !stablehash.Valid(v.BrandCTA.CTADigest) {
		return fault.Invalid("COMPOSITION_MANIFEST_BRAND_CTA_INVALID", "合成清单必须固定品牌和 CTA 配置摘要")
	}
	if err := v.Timeline.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(v.Renderer.Name) == "" || strings.TrimSpace(v.Renderer.Version) == "" || !stablehash.Valid(v.Renderer.CapabilityDigest) {
		return fault.Invalid("COMPOSITION_MANIFEST_RENDERER_INVALID", "合成清单必须固定 renderer 名称、版本和 capability 摘要")
	}
	if v.Output.Kind != "final_render" || strings.TrimSpace(v.Output.MediaType) == "" || strings.TrimSpace(v.Output.Container) == "" || strings.TrimSpace(v.Output.VideoCodec) == "" || strings.TrimSpace(v.Output.AudioCodec) == "" {
		return fault.Invalid("COMPOSITION_MANIFEST_OUTPUT_INVALID", "合成清单必须声明完整的 final_render 输出契约")
	}
	return nil
}

func (v CompositionNarration) validate() error {
	switch v.Status {
	case "disabled":
		if v.ArtifactID != "" || v.Digest != "" || v.Language != "" || v.VoiceConfigDigest != "" {
			return fault.Invalid("COMPOSITION_MANIFEST_NARRATION_INVALID", "禁用旁白时不得携带旁白 Artifact 或配置")
		}
	case "enabled":
		if strings.TrimSpace(v.ArtifactID) == "" || !stablehash.Valid(v.Digest) || strings.TrimSpace(v.Language) == "" || !stablehash.Valid(v.VoiceConfigDigest) {
			return fault.Invalid("COMPOSITION_MANIFEST_NARRATION_INVALID", "启用旁白时必须固定 Artifact、语言和声音配置摘要")
		}
	default:
		return fault.Invalid("COMPOSITION_MANIFEST_NARRATION_STATUS_INVALID", "旁白必须显式标记 enabled 或 disabled")
	}
	return nil
}

func (v CompositionSubtitles) validate() error {
	switch v.Status {
	case "disabled":
		if v.SourceKind != "none" || v.ArtifactID != "" || v.Digest != "" || v.ContentDigest != "" || v.Language != "" || v.StyleVersion != "" {
			return fault.Invalid("COMPOSITION_MANIFEST_SUBTITLES_INVALID", "禁用字幕时 source_kind 必须为 none 且不得携带字幕输入")
		}
	case "enabled":
		if v.SourceKind != "artifact" && v.SourceKind != "inline" {
			return fault.Invalid("COMPOSITION_MANIFEST_SUBTITLES_INVALID", "启用字幕时 source_kind 必须为 artifact 或 inline")
		}
		if strings.TrimSpace(v.Language) == "" || strings.TrimSpace(v.StyleVersion) == "" {
			return fault.Invalid("COMPOSITION_MANIFEST_SUBTITLES_INVALID", "启用字幕时必须固定语言和样式版本")
		}
		if v.SourceKind == "artifact" && (strings.TrimSpace(v.ArtifactID) == "" || !stablehash.Valid(v.Digest)) {
			return fault.Invalid("COMPOSITION_MANIFEST_SUBTITLES_INVALID", "Artifact 字幕必须绑定 Artifact 和摘要")
		}
		if v.SourceKind == "inline" && !stablehash.Valid(v.ContentDigest) {
			return fault.Invalid("COMPOSITION_MANIFEST_SUBTITLES_INVALID", "内嵌字幕必须固定内容摘要")
		}
	default:
		return fault.Invalid("COMPOSITION_MANIFEST_SUBTITLES_STATUS_INVALID", "字幕必须显式标记 enabled 或 disabled")
	}
	return nil
}

func (v CompositionTimeline) validate() error {
	if v.CanvasWidth <= 0 || v.CanvasHeight <= 0 || v.FrameRateNumerator <= 0 || v.FrameRateDenominator <= 0 || v.DurationMS <= 0 || strings.TrimSpace(v.TransitionVersion) == "" || len(v.Segments) == 0 {
		return fault.Invalid("COMPOSITION_MANIFEST_TIMELINE_INVALID", "时间轴必须声明画布、帧率、时长、转场版本和片段")
	}
	seen := map[string]struct{}{}
	var previousEnd int64
	for index, segment := range v.Segments {
		if strings.TrimSpace(segment.ID) == "" || strings.TrimSpace(segment.ArtifactID) == "" || !stablehash.Valid(segment.ArtifactDigest) || segment.StartMS < 0 || segment.EndMS <= segment.StartMS || segment.EndMS > v.DurationMS {
			return fault.Invalid("COMPOSITION_MANIFEST_SEGMENT_INVALID", "时间轴片段必须有有效边界、Artifact 和摘要")
		}
		if _, exists := seen[segment.ID]; exists {
			return fault.Invalid("COMPOSITION_MANIFEST_SEGMENT_DUPLICATE", "时间轴片段 ID 不能重复")
		}
		seen[segment.ID] = struct{}{}
		if index > 0 && segment.StartMS < previousEnd {
			return fault.Invalid("COMPOSITION_MANIFEST_TIMELINE_OVERLAP", "时间轴片段不能重叠或逆序")
		}
		previousEnd = segment.EndMS
	}
	return nil
}

func (v CompositionManifest) CanonicalJSON() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	value := v
	if value.Timeline.Segments == nil {
		value.Timeline.Segments = []CompositionSegment{}
	}
	return json.Marshal(value)
}

func (v CompositionManifest) Digest() (string, error) {
	payload, err := v.CanonicalJSON()
	if err != nil {
		return "", err
	}
	hash, err := stablehash.Sum(json.RawMessage(payload))
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}
