package environment

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type Registry struct {
	SchemaURL     string          `json:"$schema,omitempty"`
	SchemaVersion string          `json:"schema_version"`
	Entries       []RegistryEntry `json:"entries"`
}

type RegistryEntry struct {
	ID                 string             `json:"id"`
	Kind               string             `json:"kind"`
	Version            string             `json:"version"`
	Source             RegistrySource     `json:"source"`
	License            string             `json:"license"`
	Digest             string             `json:"digest"`
	Signature          RegistrySignature  `json:"signature"`
	CompatibleProfiles []string           `json:"compatible_profiles"`
	Permissions        []string           `json:"permissions"`
	DataFlow           RegistryDataFlow   `json:"data_flow"`
	Cost               RegistryCost       `json:"cost"`
	OutputSchemas      []string           `json:"output_schemas"`
	Evaluation         RegistryEvaluation `json:"evaluation"`
	Lifecycle          string             `json:"lifecycle"`
	Revocation         RegistryRevocation `json:"revocation"`
}

type RegistrySource struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
}

type RegistrySignature struct {
	Status    string `json:"status"`
	Algorithm string `json:"algorithm,omitempty"`
	KeyID     string `json:"key_id,omitempty"`
	Value     string `json:"value,omitempty"`
}

type RegistryDataFlow struct {
	LocalByDefault bool     `json:"local_by_default"`
	CloudActions   []string `json:"cloud_actions"`
}

type RegistryCost struct {
	Model     string `json:"model"`
	Currency  string `json:"currency,omitempty"`
	Unit      string `json:"unit,omitempty"`
	UnitPrice string `json:"unit_price,omitempty"`
	Notice    string `json:"notice"`
}

type RegistryEvaluation struct {
	Status   string   `json:"status"`
	Report   string   `json:"report,omitempty"`
	Digest   string   `json:"digest,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

type RegistryRevocation struct {
	Status   string `json:"status"`
	Severity string `json:"severity,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type RegistryPurpose string

const (
	PurposeNewInstall      RegistryPurpose = "new_install"
	PurposeNewRun          RegistryPurpose = "new_run"
	PurposeHistoricalAudit RegistryPurpose = "historical_audit"
)

type RegistryDisposition struct {
	Allowed        bool   `json:"allowed"`
	HistoricalOnly bool   `json:"historical_only"`
	Warning        string `json:"warning,omitempty"`
}

func (registry Registry) Exact(id, version, digest string) (RegistryEntry, error) {
	var found *RegistryEntry
	for index := range registry.Entries {
		entry := registry.Entries[index]
		if entry.ID != id || entry.Version != version || entry.Digest != digest {
			continue
		}
		if found != nil {
			return RegistryEntry{}, domain.Conflict("REGISTRY_ENTRY_DUPLICATED", "Marketplace Registry 包含重复的 ID/version/digest")
		}
		copy := entry
		found = &copy
	}
	if found == nil {
		return RegistryEntry{}, domain.NotFound("Marketplace Registry 精确版本")
	}
	return *found, nil
}

func AssessRegistryEntry(entry RegistryEntry, purpose RegistryPurpose) (RegistryDisposition, error) {
	if err := validateRegistryEntryMetadata(entry); err != nil {
		return RegistryDisposition{}, err
	}
	if entry.Signature.Status != "verified" || entry.Evaluation.Status != "passed" {
		return RegistryDisposition{}, domain.Policy("REGISTRY_ENTRY_UNTRUSTED", "Marketplace Registry entry 尚未通过签名和评测门禁", "仅使用 ContentCloud 已发布并签名的精选能力")
	}
	if purpose == PurposeHistoricalAudit {
		if entry.Revocation.Status == "revoked" || entry.Lifecycle == "revoked" {
			return RegistryDisposition{Allowed: true, HistoricalOnly: true, Warning: defaultRegistryReason(entry.Revocation.Reason)}, nil
		}
		return RegistryDisposition{Allowed: true}, nil
	}
	if purpose != PurposeNewInstall && purpose != PurposeNewRun {
		return RegistryDisposition{}, domain.Invalid("REGISTRY_PURPOSE_INVALID", "Marketplace Registry 使用目的无效")
	}
	if entry.Revocation.Status == "revoked" || entry.Lifecycle == "revoked" {
		code := "REGISTRY_ENTRY_REVOKED"
		message := fmt.Sprintf("Plugin %s 已撤回，禁止新安装或新 Run", entry.ID)
		if purpose == PurposeNewRun && entry.Revocation.Severity == "high" {
			code = "REGISTRY_ENTRY_HIGH_RISK_REVOKED"
			message = fmt.Sprintf("高风险 Plugin %s 已撤回，必须阻止新 Run", entry.ID)
		}
		return RegistryDisposition{}, domain.Policy(code, message, defaultRegistryReason(entry.Revocation.Reason))
	}
	if entry.Lifecycle != "published" {
		return RegistryDisposition{}, domain.Policy("REGISTRY_ENTRY_NOT_PUBLISHED", "Marketplace Registry entry 尚未发布，不能用于新安装或新 Run", "等待 ContentCloud 发布不可变版本")
	}
	return RegistryDisposition{Allowed: true}, nil
}

func validateRegistryEntryMetadata(entry RegistryEntry) error {
	if !pluginIDPattern.MatchString(entry.ID) || !validPluginKind(entry.Kind) || !versionPattern.MatchString(entry.Version) || !digestPattern.MatchString(entry.Digest) || !sourceRefPattern.MatchString(entry.Source.Ref) {
		return domain.Invalid("REGISTRY_ENTRY_INVALID", "Marketplace Registry entry 标识、版本、ref 或 digest 无效")
	}
	if strings.TrimSpace(entry.Source.Repository) == "" || strings.TrimSpace(entry.License) == "" || len(entry.Permissions) == 0 || len(entry.OutputSchemas) == 0 || len(entry.CompatibleProfiles) == 0 {
		return domain.Invalid("REGISTRY_ENTRY_METADATA_INVALID", "Marketplace Registry entry 缺少来源、license、权限、兼容 Profile 或输出 Schema")
	}
	if !contains([]string{"free", "included", "metered", "external"}, entry.Cost.Model) || strings.TrimSpace(entry.Cost.Notice) == "" {
		return domain.Invalid("REGISTRY_COST_INVALID", "Marketplace Registry entry 缺少有效费用模式或披露")
	}
	if entry.Cost.Model == "metered" && (len(entry.Cost.Currency) != 3 || strings.TrimSpace(entry.Cost.Unit) == "" || strings.TrimSpace(entry.Cost.UnitPrice) == "") {
		return domain.Invalid("REGISTRY_COST_INVALID", "metered Registry entry 必须声明 currency、unit 和 unit_price")
	}
	repository, err := url.Parse(entry.Source.Repository)
	if err != nil || repository.Scheme != "https" || repository.Host == "" || repository.User != nil || repository.RawQuery != "" || repository.Fragment != "" {
		return domain.Invalid("REGISTRY_SOURCE_INVALID", "Marketplace Registry repository 必须是无凭据、query 和 fragment 的 HTTPS URL")
	}
	if entry.Revocation.Status != "active" && entry.Revocation.Status != "revoked" {
		return domain.Invalid("REGISTRY_REVOCATION_INVALID", "Marketplace Registry 撤回状态无效")
	}
	if entry.Revocation.Status == "revoked" && (entry.Lifecycle != "revoked" || (entry.Revocation.Severity != "advisory" && entry.Revocation.Severity != "high") || strings.TrimSpace(entry.Revocation.Reason) == "") {
		return domain.Invalid("REGISTRY_REVOCATION_INVALID", "已撤回 Registry entry 必须同时声明 revoked lifecycle、风险级别和原因")
	}
	if entry.Revocation.Status == "active" && entry.Lifecycle == "revoked" {
		return domain.Invalid("REGISTRY_REVOCATION_INVALID", "revoked lifecycle 必须对应 revoked 状态")
	}
	if !contains([]string{"draft", "security_review", "evaluated", "published", "deprecated", "revoked"}, entry.Lifecycle) {
		return domain.Invalid("REGISTRY_LIFECYCLE_INVALID", "Marketplace Registry lifecycle 无效")
	}
	if entry.Evaluation.Status != "pending" && entry.Evaluation.Status != "passed" && entry.Evaluation.Status != "failed" {
		return domain.Invalid("REGISTRY_EVALUATION_INVALID", "Marketplace Registry evaluation 状态无效")
	}
	return nil
}

func defaultRegistryReason(reason string) string {
	if normalized := strings.TrimSpace(reason); normalized != "" {
		return normalized
	}
	return "查看 ContentCloud Marketplace 撤回公告"
}
