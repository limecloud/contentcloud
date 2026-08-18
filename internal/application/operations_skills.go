package application

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/catalog/environment"
)

type OperationsSkillSource struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	License    string `json:"license"`
}

type OperationsSkillSignature struct {
	Status    string `json:"status"`
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
}

type OperationsSkillDataFlow struct {
	LocalByDefault bool     `json:"local_by_default"`
	CloudActions   []string `json:"cloud_actions"`
}

type OperationsSkillCost struct {
	Model     string `json:"model"`
	Currency  string `json:"currency,omitempty"`
	Unit      string `json:"unit,omitempty"`
	UnitPrice string `json:"unit_price,omitempty"`
	Notice    string `json:"notice"`
}

type OperationsSkillEvaluation struct {
	Status   string   `json:"status"`
	Report   string   `json:"report,omitempty"`
	Digest   string   `json:"digest,omitempty"`
	Evidence []string `json:"evidence"`
}

type OperationsSkillRevocation struct {
	Status   string `json:"status"`
	Severity string `json:"severity,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type OperationsSkill struct {
	ID                  string                    `json:"id"`
	Version             string                    `json:"version"`
	Digest              string                    `json:"digest"`
	Kind                string                    `json:"kind"`
	Lifecycle           string                    `json:"lifecycle"`
	AvailableForNewRuns bool                      `json:"available_for_new_runs"`
	Source              OperationsSkillSource     `json:"source"`
	Signature           OperationsSkillSignature  `json:"signature"`
	CompatibleProfiles  []string                  `json:"compatible_profiles"`
	Permissions         []string                  `json:"permissions"`
	DataFlow            OperationsSkillDataFlow   `json:"data_flow"`
	Cost                OperationsSkillCost       `json:"cost"`
	OutputSchemas       []string                  `json:"output_schemas"`
	Evaluation          OperationsSkillEvaluation `json:"evaluation"`
	Revocation          OperationsSkillRevocation `json:"revocation"`
}

type OperationsSkillDirectory struct {
	Configured            bool              `json:"configured"`
	Source                string            `json:"source,omitempty"`
	RegistrySchemaVersion string            `json:"registry_schema_version,omitempty"`
	Skills                []OperationsSkill `json:"skills"`
	GeneratedAt           time.Time         `json:"generated_at"`
}

func (s *OperationsService) OperationsSkills(ctx context.Context, actor Actor) (OperationsSkillDirectory, error) {
	if !actor.PlatformAdmin {
		return OperationsSkillDirectory{}, fault.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以查看技能包目录", "使用已授权的平台管理员账号")
	}
	now := s.now().UTC()
	directory := OperationsSkillDirectory{Skills: []OperationsSkill{}, GeneratedAt: now}
	if s.environmentControl == nil {
		return directory, nil
	}
	registry, err := s.environmentControl.Registry()
	if err != nil {
		return OperationsSkillDirectory{}, err
	}
	directory.Configured = true
	directory.Source = "verified_plugin_registry"
	directory.RegistrySchemaVersion = registry.SchemaVersion
	for _, entry := range registry.Entries {
		if entry.Kind != "skill_pack" {
			continue
		}
		directory.Skills = append(directory.Skills, projectOperationsSkill(entry))
	}
	sort.Slice(directory.Skills, func(i, j int) bool {
		if directory.Skills[i].ID == directory.Skills[j].ID {
			return directory.Skills[i].Version < directory.Skills[j].Version
		}
		return directory.Skills[i].ID < directory.Skills[j].ID
	})
	return directory, nil
}

func (s *OperationsService) OperationsSkill(ctx context.Context, actor Actor, id, version string) (OperationsSkill, error) {
	directory, err := s.OperationsSkills(ctx, actor)
	if err != nil {
		return OperationsSkill{}, err
	}
	for _, skill := range directory.Skills {
		if skill.ID == id && skill.Version == version {
			return skill, nil
		}
	}
	return OperationsSkill{}, fault.NotFound("技能包版本")
}

func projectOperationsSkill(entry environment.RegistryEntry) OperationsSkill {
	disposition, err := environment.AssessRegistryEntry(entry, environment.PurposeNewRun)
	availableForNewRuns := err == nil && disposition.Allowed && !disposition.HistoricalOnly
	return OperationsSkill{
		ID:                  entry.ID,
		Version:             entry.Version,
		Digest:              entry.Digest,
		Kind:                entry.Kind,
		Lifecycle:           entry.Lifecycle,
		AvailableForNewRuns: availableForNewRuns,
		Source:              OperationsSkillSource{Repository: entry.Source.Repository, Ref: entry.Source.Ref, License: entry.License},
		Signature:           OperationsSkillSignature{Status: entry.Signature.Status, Algorithm: entry.Signature.Algorithm, KeyID: entry.Signature.KeyID},
		CompatibleProfiles:  append([]string{}, entry.CompatibleProfiles...),
		Permissions:         append([]string{}, entry.Permissions...),
		DataFlow:            OperationsSkillDataFlow{LocalByDefault: entry.DataFlow.LocalByDefault, CloudActions: append([]string{}, entry.DataFlow.CloudActions...)},
		Cost:                OperationsSkillCost{Model: entry.Cost.Model, Currency: entry.Cost.Currency, Unit: entry.Cost.Unit, UnitPrice: entry.Cost.UnitPrice, Notice: entry.Cost.Notice},
		OutputSchemas:       append([]string{}, entry.OutputSchemas...),
		Evaluation:          OperationsSkillEvaluation{Status: entry.Evaluation.Status, Report: entry.Evaluation.Report, Digest: entry.Evaluation.Digest, Evidence: append([]string{}, entry.Evaluation.Evidence...)},
		Revocation:          OperationsSkillRevocation{Status: entry.Revocation.Status, Severity: entry.Revocation.Severity, Reason: entry.Revocation.Reason},
	}
}
