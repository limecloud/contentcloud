package domain

import (
	"mime"
	"regexp"
	"strings"
)

const (
	ArtifactEnvelopeVersion  = "1.0"
	ReviewProjectionSchema   = "review-projection/1.0"
	ArtifactOpenCapability   = "contentcloud.artifact.open"
	ArtifactExportSchemaJSON = "contentcloud.script-export.json/1.0"
	ArtifactExportSchemaMD   = "contentcloud.script-export.markdown/1.0"
	ArtifactExportSchemaXLSX = "contentcloud.script-export.xlsx/1.0"
)

var (
	artifactSHA256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	artifactSemverPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
)

func ValidateExtensionArtifactEnvelope(value ExtensionArtifactEnvelopeV1) error {
	if value.EnvelopeVersion != ArtifactEnvelopeVersion {
		return Invalid("ARTIFACT_ENVELOPE_VERSION_INVALID", "Artifact Envelope 版本必须为 1.0")
	}
	if strings.TrimSpace(value.ProjectID) == "" || strings.TrimSpace(value.ScriptVersionID) == "" {
		return Invalid("ARTIFACT_SCOPE_REQUIRED", "Artifact Envelope 必须绑定项目和剧本版本")
	}
	if strings.TrimSpace(value.Capability.ID) == "" || !artifactSemverPattern.MatchString(value.Capability.Version) || strings.TrimSpace(value.Capability.Digest) == "" {
		return Invalid("ARTIFACT_CAPABILITY_INVALID", "Artifact capability 必须包含 ID、semver 版本和摘要")
	}
	if strings.TrimSpace(value.SchemaID) == "" || len(value.SchemaID) > 160 {
		return Invalid("ARTIFACT_SCHEMA_INVALID", "Artifact schema_id 无效")
	}
	if err := validateArtifactRef(ArtifactRef{SHA256: value.SHA256, MediaType: value.MediaType, Size: value.Size}, false); err != nil {
		return err
	}
	if value.ReviewProjection != nil {
		if err := validateArtifactRef(*value.ReviewProjection, true); err != nil {
			return err
		}
		if normalizedMediaType(value.ReviewProjection.MediaType) != "application/json" {
			return Invalid("REVIEW_PROJECTION_MEDIA_TYPE_INVALID", "Review Projection 必须使用 application/json")
		}
	}
	if len(value.Renditions) > 16 {
		return Invalid("ARTIFACT_RENDITION_LIMIT", "单个 Artifact 最多声明 16 个 rendition")
	}
	seenPurposes := map[string]bool{}
	for _, rendition := range value.Renditions {
		switch rendition.Purpose {
		case "thumbnail", "preview", "poster", "transcript":
		default:
			return Invalid("ARTIFACT_RENDITION_PURPOSE_INVALID", "rendition purpose 不在允许范围")
		}
		if seenPurposes[rendition.Purpose] {
			return Invalid("ARTIFACT_RENDITION_DUPLICATE", "同一 rendition purpose 只能声明一次")
		}
		seenPurposes[rendition.Purpose] = true
		if err := validateArtifactRef(rendition.Artifact, true); err != nil {
			return err
		}
		if !SafeRenditionMediaType(rendition.Artifact.MediaType) {
			return Invalid("ARTIFACT_RENDITION_MEDIA_TYPE_BLOCKED", "rendition 媒体类型不能安全内嵌")
		}
	}
	if len(value.Metadata) > 64 {
		return Invalid("ARTIFACT_METADATA_LIMIT", "Artifact metadata 最多包含 64 个字段")
	}
	for key, item := range value.Metadata {
		if strings.TrimSpace(key) == "" || len(key) > 80 || hasControlCharacter(key) {
			return Invalid("ARTIFACT_METADATA_KEY_INVALID", "Artifact metadata 字段名无效")
		}
		switch typed := item.(type) {
		case string:
			if len(typed) > 1000 || hasControlCharacter(typed) {
				return Invalid("ARTIFACT_METADATA_VALUE_INVALID", "Artifact metadata 字符串无效")
			}
		case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, bool:
		default:
			return Invalid("ARTIFACT_METADATA_VALUE_INVALID", "Artifact metadata 只允许字符串、数字和布尔值")
		}
	}
	return nil
}

func ValidateArtifactReviewProjection(value ArtifactReviewProjectionV1, scriptVersionID string) error {
	if value.SchemaVersion != ArtifactEnvelopeVersion || value.ScriptVersionID != scriptVersionID {
		return Invalid("REVIEW_PROJECTION_SCOPE_INVALID", "Review Projection 版本或剧本绑定无效")
	}
	if strings.TrimSpace(value.Title) == "" || len(value.Title) > 200 || len(value.Summary) > 2000 {
		return Invalid("REVIEW_PROJECTION_CONTENT_INVALID", "Review Projection 标题或摘要无效")
	}
	if len(value.Sections) == 0 || len(value.Sections) > 100 {
		return Invalid("REVIEW_PROJECTION_SECTIONS_INVALID", "Review Projection 必须包含 1 到 100 个 section")
	}
	seen := map[string]bool{}
	for _, section := range value.Sections {
		if strings.TrimSpace(section.ID) == "" || seen[section.ID] || strings.TrimSpace(section.Label) == "" || len(section.Summary) > 2000 {
			return Invalid("REVIEW_PROJECTION_SECTION_INVALID", "Review Projection section 无效或重复")
		}
		seen[section.ID] = true
		if section.ScriptPointer != "" && !ValidJSONPointer(section.ScriptPointer) {
			return Invalid("REVIEW_PROJECTION_POINTER_INVALID", "Review Projection script_pointer 不是合法 JSON Pointer")
		}
		if section.ThumbnailSHA256 != "" && !artifactSHA256Pattern.MatchString(section.ThumbnailSHA256) {
			return Invalid("REVIEW_PROJECTION_THUMBNAIL_INVALID", "Review Projection thumbnail hash 无效")
		}
		for _, warning := range section.Warnings {
			if len(warning) > 500 || hasControlCharacter(warning) {
				return Invalid("REVIEW_PROJECTION_WARNING_INVALID", "Review Projection warning 无效")
			}
		}
	}
	return nil
}

func SafeRenditionMediaType(value string) bool {
	switch normalizedMediaType(value) {
	case "image/png", "image/jpeg", "image/webp", "video/mp4", "video/webm", "application/pdf", "text/plain", "text/markdown":
		return true
	default:
		return false
	}
}

func validateArtifactRef(value ArtifactRef, rendition bool) error {
	if !artifactSHA256Pattern.MatchString(value.SHA256) {
		return Invalid("ARTIFACT_SHA256_INVALID", "Artifact sha256 必须为小写十六进制摘要")
	}
	if value.Size <= 0 {
		return Invalid("ARTIFACT_SIZE_INVALID", "Artifact size 必须大于 0")
	}
	if value.Size > 100<<30 {
		return Invalid("ARTIFACT_SIZE_LIMIT", "Artifact size 超过 100 GB 上限")
	}
	mediaType := normalizedMediaType(value.MediaType)
	if mediaType == "" || len(value.MediaType) > 160 {
		return Invalid("ARTIFACT_MEDIA_TYPE_INVALID", "Artifact media_type 无效")
	}
	if rendition && (mediaType == "text/html" || mediaType == "image/svg+xml") {
		return Invalid("ARTIFACT_ACTIVE_CONTENT_BLOCKED", "普通 HTML 或 SVG 不能作为安全 rendition")
	}
	return nil
}

func normalizedMediaType(value string) string {
	parsed, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed)
}

func hasControlCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 && character != '\t' {
			return true
		}
	}
	return false
}
