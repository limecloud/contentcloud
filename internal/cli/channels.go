package cli

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (r *Root) channelCommand() *cobra.Command {
	command := &cobra.Command{Use: "channel", Short: "管理渠道账号绑定、发布意图和外部回执"}
	command.AddCommand(r.simpleUserReadCommand("adapters", "channel.adapter.list", "列出已配置渠道适配器", map[string]any{}))

	var projectID, channel, adapterID, accountRef, secretRef, region string
	bind := &cobra.Command{Use: "bind", Short: "创建只保存 SecretRef 的渠道账号绑定", RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ChannelBinding
		input := app.CreateChannelBindingInput{ProjectID: projectID, Channel: channel, AdapterID: adapterID, AccountRef: accountRef, AuthorizationSecretRef: secretRef, Region: region}
		if err := client.Dispatch(cmd.Context(), "channel.binding.create", input, &result); err != nil {
			return err
		}
		return r.writeOK("channel.binding.create", result)
	}}
	bind.Flags().StringVar(&projectID, "project", "", "项目 ID")
	bind.Flags().StringVar(&channel, "channel", "", "渠道标识，例如 wechat_official_account")
	bind.Flags().StringVar(&adapterID, "adapter", "manual", "渠道适配器 ID")
	bind.Flags().StringVar(&accountRef, "account", "", "外部账号引用")
	bind.Flags().StringVar(&secretRef, "authorization-secret-ref", "", "授权凭据在 Secret Store 中的引用")
	bind.Flags().StringVar(&region, "region", "global", "账号区域")
	_ = bind.MarkFlagRequired("project")
	_ = bind.MarkFlagRequired("channel")
	_ = bind.MarkFlagRequired("account")
	_ = bind.MarkFlagRequired("authorization-secret-ref")

	bindings := r.simpleUserReadCommand("bindings", "channel.binding.list", "列出项目渠道绑定", map[string]any{})
	bindings.Flags().StringVar(&projectID, "project", "", "项目 ID")
	_ = bindings.MarkFlagRequired("project")
	bindings.PreRunE = func(cmd *cobra.Command, _ []string) error { return nil }
	bindings.RunE = func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.ChannelBinding
		if err := client.Dispatch(cmd.Context(), "channel.binding.list", map[string]any{"project_id": projectID}, &result); err != nil {
			return err
		}
		return r.writeOK("channel.binding.list", result)
	}

	var taskDeliveryID, bindingID, idempotencyKey, schedule, contentProfileID, douyinRefsFile string
	prepare := &cobra.Command{Use: "prepare", Short: "固定交付包、账号和最终预览", RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var scheduledAt *time.Time
		if schedule != "" {
			parsed, err := time.Parse(time.RFC3339, schedule)
			if err != nil {
				return domain.Invalid("CHANNEL_SCHEDULE_INVALID", "--schedule 必须是 RFC3339 时间")
			}
			scheduledAt = &parsed
		}
		var result domain.ChannelPublication
		input := app.PrepareChannelPublicationInput{TaskDeliveryID: taskDeliveryID, ChannelBindingID: bindingID, IdempotencyKey: idempotencyKey, ContentProfileID: contentProfileID, ScheduledAt: scheduledAt, Metadata: map[string]any{}}
		if strings.TrimSpace(douyinRefsFile) != "" {
			body, err := os.ReadFile(douyinRefsFile)
			if err != nil {
				return err
			}
			var refs domain.DouyinCommercePublicationRefs
			if err := json.Unmarshal(body, &refs); err != nil {
				return domain.Invalid("DOUYIN_COMMERCE_REFS_JSON_INVALID", "--douyin-refs 必须是有效的 DouyinCommercePublicationRefs JSON")
			}
			input.DouyinCommerce = &refs
			if input.ContentProfileID == "" {
				input.ContentProfileID = domain.DouyinCommerceProfileID
			}
		}
		if err := client.Dispatch(cmd.Context(), "channel.publication.prepare", input, &result); err != nil {
			return err
		}
		return r.writeOK("channel.publication.prepare", result)
	}}
	prepare.Flags().StringVar(&taskDeliveryID, "task-delivery", "", "ready 的任务交付 ID")
	prepare.Flags().StringVar(&bindingID, "binding", "", "渠道绑定 ID")
	prepare.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "稳定幂等键")
	prepare.Flags().StringVar(&schedule, "schedule", "", "可选 RFC3339 排期")
	prepare.Flags().StringVar(&contentProfileID, "content-profile", "", "内容 Profile ID，例如 douyin-commerce-video")
	prepare.Flags().StringVar(&douyinRefsFile, "douyin-refs", "", "抖音电商类型化发布引用 JSON 文件")
	_ = prepare.MarkFlagRequired("task-delivery")
	_ = prepare.MarkFlagRequired("binding")
	_ = prepare.MarkFlagRequired("idempotency-key")

	command.AddCommand(bind, bindings, prepare, r.channelPublicationAction("submit", "channel.publication.submit", true), r.channelPublicationAction("inspect", "channel.publication.inspect", false), r.channelPublicationReceiptCommand(), r.channelPublicationWithdrawCommand(), r.channelPublicationListCommand(), r.channelReconcileCommand(), r.channelPerformanceCommand())
	return command
}

func (r *Root) channelReconcileCommand() *cobra.Command {
	var limit int
	command := &cobra.Command{Use: "reconcile", Short: "批量检查 submitted 和 unknown 发布的外部状态", RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result app.ChannelReconcileResult
		if err := client.Dispatch(cmd.Context(), "channel.publication.reconcile", map[string]any{"limit": limit}, &result); err != nil {
			return err
		}
		return r.writeOK("channel.publication.reconcile", result)
	}}
	command.Flags().IntVar(&limit, "limit", 100, "单次最多检查的发布数量")
	return command
}

func (r *Root) channelPerformanceCommand() *cobra.Command {
	var snapshotID, sampleStatus, currency, issueCategory, notes string
	var windowHours int
	var metricFlags []string
	var spend, gmv float64
	command := &cobra.Command{Use: "performance <publication-id>", Args: cobra.ExactArgs(1), Short: "把 published 回执绑定的渠道指标导入 PerformanceObservation", RunE: func(cmd *cobra.Command, args []string) error {
		metrics := map[string]float64{}
		for _, raw := range metricFlags {
			parts := strings.SplitN(raw, "=", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
				return domain.Invalid("CHANNEL_METRIC_INVALID", "--metric 必须使用 name=value")
			}
			value, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return domain.Invalid("CHANNEL_METRIC_INVALID", "--metric value 必须是数字")
			}
			metrics[strings.TrimSpace(parts[0])] = value
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		input := app.ImportChannelPerformanceInput{ApprovedSnapshotID: snapshotID, WindowHours: windowHours, SampleStatus: sampleStatus, Metrics: metrics, Currency: currency, Spend: spend, GMV: gmv, IssueCategory: issueCategory, Notes: notes}
		var result app.ImportPerformanceResult
		params := map[string]any{"id": args[0], "approved_snapshot_id": input.ApprovedSnapshotID, "window_hours": input.WindowHours, "sample_status": input.SampleStatus, "metrics": input.Metrics, "currency": input.Currency, "spend": input.Spend, "gmv": input.GMV, "issue_category": input.IssueCategory, "notes": input.Notes}
		if err := client.Dispatch(cmd.Context(), "channel.performance.import", params, &result); err != nil {
			return err
		}
		return r.writeOK("channel.performance.import", result)
	}}
	command.Flags().StringVar(&snapshotID, "approved-snapshot", "", "发布内容对应的 ApprovedSnapshot ID")
	command.Flags().IntVar(&windowHours, "window-hours", 24, "指标观察窗口小时数")
	command.Flags().StringVar(&sampleStatus, "sample-status", "insufficient_sample", "样本状态")
	command.Flags().StringSliceVar(&metricFlags, "metric", nil, "指标 name=value，可重复传入")
	command.Flags().StringVar(&currency, "currency", "", "币种")
	command.Flags().Float64Var(&spend, "spend", 0, "消耗")
	command.Flags().Float64Var(&gmv, "gmv", 0, "GMV")
	command.Flags().StringVar(&issueCategory, "issue-category", "", "归因分类")
	command.Flags().StringVar(&notes, "notes", "", "观察备注")
	_ = command.MarkFlagRequired("approved-snapshot")
	return command
}

func (r *Root) channelPublicationAction(use, dispatch string, confirm bool) *cobra.Command {
	var yes bool
	command := &cobra.Command{Use: use + " <publication-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if confirm && !yes {
			return confirmationRequired("该操作会向外部渠道提交固定交付包")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ChannelPublication
		if err := client.Dispatch(cmd.Context(), dispatch, map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK(dispatch, result)
	}}
	if confirm {
		command.Flags().BoolVar(&yes, "yes", false, "确认外部副作用")
	}
	return command
}

func (r *Root) channelPublicationReceiptCommand() *cobra.Command {
	var state, externalID, externalURL, publishedAt, errorCode string
	command := &cobra.Command{Use: "receipt <publication-id>", Args: cobra.ExactArgs(1), Short: "记录人工渠道发布证明", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var published *time.Time
		if publishedAt != "" {
			parsed, err := time.Parse(time.RFC3339, publishedAt)
			if err != nil {
				return domain.Invalid("CHANNEL_PUBLISHED_AT_INVALID", "--published-at 必须是 RFC3339 时间")
			}
			published = &parsed
		}
		var result domain.ChannelPublication
		params := map[string]any{"id": args[0], "state": state, "external_id": externalID, "external_url": externalURL, "published_at": published, "error_code": errorCode, "safe_summary": map[string]any{}}
		if err := client.Dispatch(cmd.Context(), "channel.publication.receipt", params, &result); err != nil {
			return err
		}
		return r.writeOK("channel.publication.receipt", result)
	}}
	command.Flags().StringVar(&state, "state", "published", "published、failed 或 withdrawn")
	command.Flags().StringVar(&externalID, "external-id", "", "渠道内容 ID")
	command.Flags().StringVar(&externalURL, "external-url", "", "渠道内容 URL")
	command.Flags().StringVar(&publishedAt, "published-at", "", "RFC3339 发布时间")
	command.Flags().StringVar(&errorCode, "error-code", "", "失败错误码")
	return command
}

func (r *Root) channelPublicationWithdrawCommand() *cobra.Command {
	var reason string
	var yes bool
	command := &cobra.Command{Use: "withdraw <publication-id>", Args: cobra.ExactArgs(1), Short: "撤回外部渠道内容", RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return confirmationRequired("撤回会改变外部渠道内容状态")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ChannelPublication
		if err := client.Dispatch(cmd.Context(), "channel.publication.withdraw", map[string]any{"id": args[0], "reason": reason}, &result); err != nil {
			return err
		}
		return r.writeOK("channel.publication.withdraw", result)
	}}
	command.Flags().StringVar(&reason, "reason", "", "撤回原因")
	command.Flags().BoolVar(&yes, "yes", false, "确认外部副作用")
	_ = command.MarkFlagRequired("reason")
	return command
}

func (r *Root) channelPublicationListCommand() *cobra.Command {
	var taskID string
	command := &cobra.Command{Use: "list", Short: "列出渠道发布和最新回执", RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.ChannelPublication
		if err := client.Dispatch(cmd.Context(), "channel.publication.list", map[string]any{"task_id": taskID}, &result); err != nil {
			return err
		}
		return r.writeOK("channel.publication.list", result)
	}}
	command.Flags().StringVar(&taskID, "task", "", "可选任务 ID")
	return command
}

func (r *Root) simpleUserReadCommand(use, dispatch, short string, params map[string]any) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result any
		if err := client.Dispatch(cmd.Context(), dispatch, params, &result); err != nil {
			return err
		}
		return r.writeOK(dispatch, result)
	}}
}
