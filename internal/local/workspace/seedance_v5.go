package localworkspace

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	"github.com/limecloud/contentcloud/internal/work"
)

var dynamicOfferTextPattern = regexp.MustCompile(`(?:[¥￥]|[0-9]+(?:\.[0-9]+)?\s*元|到手价|优惠券|满[0-9]+减[0-9]+|限时(?:价|优惠)|库存仅|赠品)`)

type ExportSeedancePackageOptions struct {
	Root                   string
	StoryboardSnapshotID   string
	StoryboardPackageID    string
	PackageID              string
	ProviderProfileVersion string
	AdapterCapability      work.CapabilityRef
	Mode                   string
	AspectRatio            string
	Sound                  string
	MinDurationSeconds     int
	MaxDurationSeconds     int
	MaxImages              int
	MaxVideos              int
	MaxAudios              int
}

type ExportSeedancePackageResult struct {
	Directory   string                     `json:"directory"`
	PackagePath string                     `json:"package_path"`
	ReadmePath  string                     `json:"readme_path"`
	PromptPaths []string                   `json:"prompt_paths"`
	Package     work.SeedancePromptPackage `json:"package"`
}

func ExportSeedancePackage(options ExportSeedancePackageOptions) (ExportSeedancePackageResult, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return ExportSeedancePackageResult{}, err
	}
	if options.PackageID == "" {
		options.PackageID = idgen.New()
	}
	if localSafeName(options.PackageID) != options.PackageID {
		return ExportSeedancePackageResult{}, fault.Invalid("SEEDANCE_PACKAGE_ID_INVALID", "Seedance 包 ID 只能包含字母、数字、点、下划线和连字符")
	}
	if strings.TrimSpace(options.ProviderProfileVersion) == "" || options.MinDurationSeconds < 1 || options.MaxDurationSeconds < options.MinDurationSeconds || options.MaxImages < 1 || options.MaxVideos < 0 || options.MaxAudios < 0 {
		return ExportSeedancePackageResult{}, fault.Invalid("SEEDANCE_PROVIDER_PROFILE_REQUIRED", "导出必须提供已验证 profile 版本及正确定义的时长和素材上限")
	}
	if err := options.AdapterCapability.Validate(); err != nil {
		return ExportSeedancePackageResult{}, err
	}
	storyboard, err := LoadLockedStoryboardSnapshot(root, options.StoryboardSnapshotID, options.StoryboardPackageID)
	if err != nil {
		return ExportSeedancePackageResult{}, err
	}
	uploads, assetByID, counts, err := seedanceUploads(storyboard)
	if err != nil {
		return ExportSeedancePackageResult{}, err
	}
	if counts["image"] > options.MaxImages || counts["video"] > options.MaxVideos || counts["audio"] > options.MaxAudios {
		return ExportSeedancePackageResult{}, fault.Policy("SEEDANCE_PROVIDER_LIMIT_EXCEEDED", "锁定分镜素材数量超过当前服务商配置上限", "拆分分镜包或更新经人工验证的服务商配置")
	}
	durationSeconds, segments, promptFiles, err := seedanceSegments(storyboard, uploads, assetByID, options.MinDurationSeconds, options.MaxDurationSeconds, options.AspectRatio, options.Sound)
	if err != nil {
		return ExportSeedancePackageResult{}, err
	}
	// OfferChecked is true because compilation rejects dynamic offer text and defers it to post-production.
	value := work.SeedancePromptPackage{
		ID: options.PackageID, Type: "seedance_prompt_package", SchemaVersion: work.SeedancePromptPackageSchema,
		StoryboardSnapshotID: options.StoryboardSnapshotID, StoryboardPackageID: storyboard.ID, StoryboardLockedDigest: storyboard.LockedDigest,
		Provider: "seedance", ProviderProfileVersion: strings.TrimSpace(options.ProviderProfileVersion), AdapterCapability: options.AdapterCapability,
		Mode: defaultStringV5(options.Mode, "all_reference"), Settings: work.SeedanceSettings{AspectRatio: defaultStringV5(options.AspectRatio, "9:16"), DurationSeconds: durationSeconds, Sound: defaultStringV5(options.Sound, "environment_only")},
		UploadManifest: uploads, Segments: segments,
		PostProductionPlan: []string{"按原剧本时长裁切各段并完成连续性剪辑", "后期合成已批准字幕、品牌 LOGO、CTA 和必要免责声明", "涉及价格、优惠、赠品或库存时，在最终渲染和抖音发布前重新校验 CommerceOfferSnapshot"},
		Validation:         work.SeedanceValidation{ReferencesChecked: true, LimitsChecked: true, RightsChecked: true, OfferChecked: true, DigestChecked: true}, Status: "validated",
	}
	if err := value.Validate(); err != nil {
		return ExportSeedancePackageResult{}, err
	}
	base := filepath.Join(root, "60-delivery", "packages", value.ID, "providers")
	finalDirectory := filepath.Join(base, "seedance")
	if _, err := os.Stat(finalDirectory); err == nil {
		return ExportSeedancePackageResult{}, fault.Conflict("SEEDANCE_PACKAGE_EXISTS", "Seedance 交付目录已存在，拒绝覆盖不可变交付包")
	} else if !os.IsNotExist(err) {
		return ExportSeedancePackageResult{}, err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return ExportSeedancePackageResult{}, err
	}
	temporary, err := os.MkdirTemp(base, ".seedance-*")
	if err != nil {
		return ExportSeedancePackageResult{}, err
	}
	defer os.RemoveAll(temporary)
	if err := writeSeedancePackageFiles(root, temporary, storyboard, value, assetByID, promptFiles); err != nil {
		return ExportSeedancePackageResult{}, err
	}
	if err := os.Rename(temporary, finalDirectory); err != nil {
		return ExportSeedancePackageResult{}, err
	}
	promptPaths := make([]string, len(promptFiles))
	for index := range promptFiles {
		promptPaths[index] = relativeWorkspacePath(root, filepath.Join(finalDirectory, "prompts", fmt.Sprintf("segment-%02d.txt", index+1)))
	}
	return ExportSeedancePackageResult{
		Directory: relativeWorkspacePath(root, finalDirectory), PackagePath: relativeWorkspacePath(root, filepath.Join(finalDirectory, "package.json")),
		ReadmePath: relativeWorkspacePath(root, filepath.Join(finalDirectory, "README.md")), PromptPaths: promptPaths, Package: value,
	}, nil
}

func LintSeedancePackage(root, file string) (V5LintReport, work.SeedancePromptPackage, error) {
	resolved, path, err := resolveV5JSON(root, file)
	if err != nil {
		return V5LintReport{}, work.SeedancePromptPackage{}, err
	}
	var value work.SeedancePromptPackage
	if err := readStrictJSON(path, &value); err != nil {
		return V5LintReport{}, value, fault.Invalid("SEEDANCE_PACKAGE_JSON_INVALID", err.Error())
	}
	report := v5Report(resolved, path, value.ID, value.Type)
	if err := value.Validate(); err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(err), Message: err.Error()})
	}
	if _, err := LoadLockedStoryboardSnapshot(resolved, value.StoryboardSnapshotID, value.StoryboardPackageID); err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(err), Message: err.Error()})
	}
	packageDirectory := filepath.Dir(path)
	for _, upload := range value.UploadManifest {
		absolute, err := ResolveWorkspaceFile(resolved, filepath.Join(packageDirectory, filepath.FromSlash(upload.File)))
		if err != nil {
			report.Issues = append(report.Issues, V5LintIssue{Code: "SEEDANCE_MEDIA_PATH_INVALID", Message: err.Error()})
			continue
		}
		sha, _, err := fileDigest(absolute)
		if err != nil {
			report.Issues = append(report.Issues, V5LintIssue{Code: "SEEDANCE_MEDIA_UNREADABLE", Message: upload.File + ": " + err.Error()})
			continue
		}
		if sha != upload.SHA256 {
			report.Issues = append(report.Issues, V5LintIssue{Code: "SEEDANCE_MEDIA_DIGEST_MISMATCH", Message: "交付媒体与 upload manifest 摘要不一致：" + upload.File})
		}
	}
	for index, segment := range value.Segments {
		promptPath := filepath.Join(packageDirectory, "prompts", fmt.Sprintf("segment-%02d.txt", index+1))
		body, err := os.ReadFile(promptPath)
		if err != nil {
			report.Issues = append(report.Issues, V5LintIssue{Code: "SEEDANCE_PROMPT_UNREADABLE", Message: relativeWorkspacePath(resolved, promptPath) + ": " + err.Error()})
			continue
		}
		if strings.TrimSpace(string(body)) != strings.TrimSpace(segment.PromptZH) {
			report.Issues = append(report.Issues, V5LintIssue{Code: "SEEDANCE_PROMPT_MISMATCH", Message: "提示词文件与 package.json 不一致：" + relativeWorkspacePath(resolved, promptPath)})
		}
	}
	report.LockedDigest = value.StoryboardLockedDigest
	report = finishV5Report(report, value)
	return report, value, nil
}

func seedanceUploads(storyboard work.StoryboardPackage) ([]work.SeedanceUpload, map[string]work.StoryboardAsset, map[string]int, error) {
	assets := map[string]work.StoryboardAsset{}
	for _, asset := range storyboard.Assets {
		assets[asset.ID] = asset
	}
	ordered := []work.StoryboardAsset{}
	seen := map[string]bool{}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		if asset, ok := assets[id]; ok && asset.Role != "review_sheet" {
			ordered = append(ordered, asset)
			seen[id] = true
		}
	}
	common := []work.StoryboardAsset{}
	for _, asset := range storyboard.Assets {
		if asset.Role == "identity_anchor" || asset.Role == "reference_video" || asset.Role == "reference_audio" {
			common = append(common, asset)
		}
	}
	sort.Slice(common, func(i, j int) bool {
		if common[i].Role != common[j].Role {
			return common[i].Role < common[j].Role
		}
		return common[i].ID < common[j].ID
	})
	for _, asset := range common {
		add(asset.ID)
	}
	shots := append([]work.StoryboardShot(nil), storyboard.Shots...)
	sort.Slice(shots, func(i, j int) bool {
		if shots[i].StartMS != shots[j].StartMS {
			return shots[i].StartMS < shots[j].StartMS
		}
		return shots[i].ShotID < shots[j].ShotID
	})
	for _, shot := range shots {
		add(shot.FirstFrameArtifactID)
		add(shot.EndFrameArtifactID)
	}
	counters := map[string]int{"image": 0, "video": 0, "audio": 0}
	uploads := make([]work.SeedanceUpload, 0, len(ordered))
	for _, asset := range ordered {
		if len(asset.RightsRefs) == 0 {
			return nil, nil, nil, fault.Policy("SEEDANCE_RIGHTS_REQUIRED", "Seedance 输入素材缺少权利记录："+asset.Path, "补齐权利记录并重新发布分镜审核")
		}
		kind, label, prefix := seedanceMediaKind(asset.MediaType)
		if kind == "" {
			return nil, nil, nil, fault.Invalid("SEEDANCE_MEDIA_TYPE_UNSUPPORTED", "Seedance 输入素材类型不受支持："+asset.MediaType)
		}
		counters[kind]++
		extension := strings.ToLower(filepath.Ext(asset.Path))
		file := fmt.Sprintf("media/%s-%02d%s", prefix, counters[kind], extension)
		uploads = append(uploads, work.SeedanceUpload{Reference: fmt.Sprintf("@%s%d", label, counters[kind]), ArtifactID: asset.ID, File: file, Purpose: seedanceAssetPurpose(asset), SHA256: asset.SHA256})
	}
	return uploads, assets, counters, nil
}

func seedanceSegments(storyboard work.StoryboardPackage, uploads []work.SeedanceUpload, assets map[string]work.StoryboardAsset, minSeconds, maxSeconds int, aspectRatio, sound string) (int, []work.SeedanceSegment, []string, error) {
	referenceByAsset := map[string]string{}
	common := []string{}
	for _, upload := range uploads {
		referenceByAsset[upload.ArtifactID] = upload.Reference
		asset := assets[upload.ArtifactID]
		if asset.Role == "identity_anchor" || asset.Role == "reference_video" || asset.Role == "reference_audio" {
			common = append(common, upload.Reference)
		}
	}
	shots := append([]work.StoryboardShot(nil), storyboard.Shots...)
	sort.Slice(shots, func(i, j int) bool {
		if shots[i].StartMS != shots[j].StartMS {
			return shots[i].StartMS < shots[j].StartMS
		}
		return shots[i].ShotID < shots[j].ShotID
	})
	requestedDuration := minSeconds
	segments := make([]work.SeedanceSegment, 0, len(shots))
	prompts := make([]string, 0, len(shots))
	for index, shot := range shots {
		if dynamicOfferTextPattern.MatchString(strings.Join([]string{shot.ImagePromptZH, shot.Subject, shot.Product, shot.Scene, shot.Action}, " ")) {
			return 0, nil, nil, fault.Policy("SEEDANCE_DYNAMIC_OFFER_TEXT_BLOCKED", "镜头 "+shot.ShotID+" 包含价格、优惠、库存或赠品等动态权益文本", "从生成画面移除动态权益，并在有效 OfferSnapshot 校验后的后期阶段合成")
		}
		shotSeconds := int(math.Ceil(float64(shot.EndMS-shot.StartMS) / 1000))
		if shotSeconds > maxSeconds {
			return 0, nil, nil, fault.Policy("SEEDANCE_SEGMENT_TOO_LONG", "镜头 "+shot.ShotID+" 超过当前 provider profile 的单段时长", "先在规范剧本中按叙事动作拆镜并重新审核分镜")
		}
		if shotSeconds > requestedDuration {
			requestedDuration = shotSeconds
		}
		refs := append([]string(nil), common...)
		refs = appendReference(refs, referenceByAsset[shot.FirstFrameArtifactID])
		refs = appendReference(refs, referenceByAsset[shot.EndFrameArtifactID])
		if len(refs) == 0 {
			return 0, nil, nil, fault.Invalid("SEEDANCE_SHOT_REFERENCE_REQUIRED", "镜头 "+shot.ShotID+" 没有可导出的首尾帧或公共参考")
		}
		prompt := compileSeedancePrompt(shot, refs, referenceByAsset[shot.FirstFrameArtifactID], referenceByAsset[shot.EndFrameArtifactID], defaultStringV5(aspectRatio, "9:16"), defaultStringV5(sound, "environment_only"), minSeconds)
		segments = append(segments, work.SeedanceSegment{ID: fmt.Sprintf("segment-%02d", index+1), Order: index + 1, StartMS: shot.StartMS, EndMS: shot.EndMS, PromptZH: prompt, IncomingState: shot.IncomingState, OutgoingState: shot.OutgoingState, AcceptanceCriteria: append([]string(nil), shot.AcceptanceCriteria...)})
		prompts = append(prompts, prompt+"\n")
	}
	if requestedDuration > maxSeconds {
		requestedDuration = maxSeconds
	}
	return requestedDuration, segments, prompts, nil
}

func compileSeedancePrompt(shot work.StoryboardShot, refs []string, firstRef, endRef, aspectRatio, sound string, minSeconds int) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s 竖屏视频，按已审核分镜生成。\n", aspectRatio)
	fmt.Fprintf(&builder, "参考素材：%s。", strings.Join(refs, "、"))
	if firstRef != "" {
		fmt.Fprintf(&builder, "%s 是镜头首帧", firstRef)
	}
	if endRef != "" {
		fmt.Fprintf(&builder, "，%s 是镜头尾帧", endRef)
	}
	builder.WriteString("。\n")
	fmt.Fprintf(&builder, "入场状态：%s。主体与场景：%s；%s；%s。\n", shot.IncomingState, shot.Subject, shot.Product, shot.Scene)
	fmt.Fprintf(&builder, "视觉基准：%s。\n", shot.ImagePromptZH)
	fmt.Fprintf(&builder, "0.0-%0.1f 秒：%s。构图与光线：%s，%s。运镜：%s。\n", float64(shot.EndMS-shot.StartMS)/1000, shot.Action, shot.Composition, shot.Lighting, shot.Camera)
	fmt.Fprintf(&builder, "声音意图：%s。\n", sound)
	if shot.EndMS-shot.StartMS < minSeconds*1000 {
		fmt.Fprintf(&builder, "动作完成后自然保持输出状态，生成后按原镜头 %0.1f 秒裁切。\n", float64(shot.EndMS-shot.StartMS)/1000)
	}
	fmt.Fprintf(&builder, "输出状态：%s。连续性锁：运动轴 %s；光线 %s；商品 %s；锚点 %s。\n", shot.OutgoingState, shot.MovementAxis, shot.LightingLock, shot.ProductLock, strings.Join(shot.Anchors, "、"))
	fmt.Fprintf(&builder, "禁止：%s；不生成字幕、价格、优惠、LOGO、CTA、水印或法律说明。", strings.Join(shot.NegativeConstraints, "；"))
	return builder.String()
}

func writeSeedancePackageFiles(root, directory string, storyboard work.StoryboardPackage, value work.SeedancePromptPackage, assets map[string]work.StoryboardAsset, prompts []string) error {
	if err := writeJSON(filepath.Join(directory, "package.json"), value); err != nil {
		return err
	}
	for index, prompt := range prompts {
		if err := writeNewFile(filepath.Join(directory, "prompts", fmt.Sprintf("segment-%02d.txt", index+1)), []byte(prompt)); err != nil {
			return err
		}
	}
	for _, upload := range value.UploadManifest {
		asset := assets[upload.ArtifactID]
		source, err := ResolveWorkspaceFile(root, asset.Path)
		if err != nil {
			return err
		}
		if err := copySeedanceFile(source, filepath.Join(directory, filepath.FromSlash(upload.File))); err != nil {
			return err
		}
	}
	return writeNewFile(filepath.Join(directory, "README.md"), []byte(seedanceReadme(storyboard, value)))
}

func seedanceReadme(storyboard work.StoryboardPackage, value work.SeedancePromptPackage) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Seedance 生成包 %s\n\n", value.ID)
	fmt.Fprintf(&builder, "- Storyboard ApprovedSnapshot: `%s`\n", value.StoryboardSnapshotID)
	fmt.Fprintf(&builder, "- Locked digest: `%s`\n", value.StoryboardLockedDigest)
	fmt.Fprintf(&builder, "- StoryboardPackage: `%s`\n", storyboard.ID)
	fmt.Fprintf(&builder, "- Provider profile: `%s`\n", value.ProviderProfileVersion)
	fmt.Fprintf(&builder, "- Adapter: `%s@%s`\n", value.AdapterCapability.ID, value.AdapterCapability.Version)
	fmt.Fprintf(&builder, "- Adapter digest: `%s`\n", value.AdapterCapability.Digest)
	fmt.Fprintf(&builder, "- Mode: `%s`\n- Aspect ratio: `%s`\n- Sound: `%s`\n- Generate each segment at: `%d` seconds\n\n", value.Mode, value.Settings.AspectRatio, value.Settings.Sound, value.Settings.DurationSeconds)
	builder.WriteString("## 上传顺序\n\n")
	for index, upload := range value.UploadManifest {
		fmt.Fprintf(&builder, "%d. `%s` -> `%s`，%s，SHA-256 `%s`\n", index+1, upload.File, upload.Reference, upload.Purpose, upload.SHA256)
	}
	builder.WriteString("\n上传完成后，先核对 Seedance 界面显示的编号与本清单一致，再逐段复制提示词。\n\n## 分段提示词\n\n")
	for index, segment := range value.Segments {
		fmt.Fprintf(&builder, "%d. `prompts/segment-%02d.txt`，原片时间 %0.1f-%0.1f 秒；入场 `%s`；输出 `%s`。\n", index+1, index+1, float64(segment.StartMS)/1000, float64(segment.EndMS)/1000, segment.IncomingState, segment.OutgoingState)
		fmt.Fprintf(&builder, "   验收：%s。\n", strings.Join(segment.AcceptanceCriteria, "；"))
	}
	builder.WriteString("\n## 人工验收\n\n生成后逐段核对商品真实性、主体一致性、首尾状态、运动轴、光线、画面安全区和验收条件。失败时只重试对应 segment，不修改已锁定分镜。\n\n## 后期与发布\n\n")
	for _, item := range value.PostProductionPlan {
		fmt.Fprintf(&builder, "- %s\n", item)
	}
	builder.WriteString("\n本包不执行 Seedance 上传、生成、下载或抖音发布；这些动作由用户在对应外部平台确认。\n")
	return builder.String()
}

func copySeedanceFile(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func seedanceMediaKind(mediaType string) (string, string, string) {
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return "image", "图片", "image"
	case strings.HasPrefix(mediaType, "video/"):
		return "video", "视频", "video"
	case strings.HasPrefix(mediaType, "audio/"):
		return "audio", "音频", "audio"
	default:
		return "", "", ""
	}
}

func seedanceAssetPurpose(asset work.StoryboardAsset) string {
	if asset.ShotID == "" {
		return asset.Role
	}
	return asset.ShotID + " " + asset.Role
}

func appendReference(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func defaultStringV5(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
