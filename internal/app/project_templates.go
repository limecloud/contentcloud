package app

import (
	"context"
	"errors"

	"github.com/limecloud/contentcloud/internal/domain"
)

type builtinProjectTemplate struct {
	Name           string
	Channel        string
	StageObjective string
}

func builtinProjectTemplates() []builtinProjectTemplate {
	return []builtinProjectTemplate{
		{Name: "短视频内容验证", Channel: "douyin", StageObjective: "验证核心卖点与内容转化"},
		{Name: "小红书种草内容", Channel: "xiaohongshu", StageObjective: "验证用户痛点与种草表达"},
		{Name: "视频号品牌内容", Channel: "wechat_channels", StageObjective: "验证品牌叙事与信任积累"},
	}
}

func (s *Service) ensureBuiltinProjectTemplates(ctx context.Context, actor Actor) error {
	existing, err := s.store.ProjectTemplates(ctx, actor.TenantID)
	if err != nil {
		return err
	}
	known := make(map[string]struct{}, len(existing))
	for _, template := range existing {
		known[template.Name] = struct{}{}
	}
	for _, preset := range builtinProjectTemplates() {
		if _, found := known[preset.Name]; found {
			continue
		}
		template := domain.ProjectTemplate{
			ID:             domain.NewID(),
			TenantID:       actor.TenantID,
			Name:           preset.Name,
			Channel:        preset.Channel,
			StageObjective: preset.StageObjective,
			CreatedBy:      actor.UserID,
			CreatedAt:      s.now().UTC(),
		}
		if err := s.store.CreateProjectTemplate(ctx, template); err != nil {
			var domainErr *domain.Error
			if errors.As(err, &domainErr) && (domainErr.Code == "PROJECT_TEMPLATE_EXISTS" || domainErr.Code == "RESOURCE_CONFLICT") {
				known[preset.Name] = struct{}{}
				continue
			}
			return err
		}
		known[preset.Name] = struct{}{}
	}
	return nil
}
