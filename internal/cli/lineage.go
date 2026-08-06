package cli

import (
	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (r *Root) lineageCommand() *cobra.Command {
	command := &cobra.Command{Use: "lineage", Short: "追踪项目对象从来源到结果的完整链路"}
	command.AddCommand(r.lineageReadCommand("show", "lineage.show", "显示双向对象链路图", true))
	command.AddCommand(r.lineageReadCommand("impact", "lineage.impact", "显示下游影响和建议操作", false))
	return command
}

func (r *Root) lineageReadCommand(use, dispatch, short string, directionFlag bool) *cobra.Command {
	var focusType, focusID, direction string
	command := &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		projectID, err := r.resolveProject(cmd, cfg, client)
		if err != nil {
			return err
		}
		query := app.LineageQuery{FocusType: focusType, FocusID: focusID, Direction: direction}
		params := struct {
			ProjectID string `json:"project_id"`
			app.LineageQuery
		}{ProjectID: projectID, LineageQuery: query}
		if dispatch == "lineage.impact" {
			var result domain.ImpactAnalysis
			if err := client.Dispatch(cmd.Context(), dispatch, params, &result); err != nil {
				return err
			}
			return r.writeOK(dispatch, result)
		}
		var result domain.LineageGraph
		if err := client.Dispatch(cmd.Context(), dispatch, params, &result); err != nil {
			return err
		}
		return r.writeOK(dispatch, result)
	}}
	command.Flags().StringVar(&focusType, "type", "", "焦点对象类型")
	command.Flags().StringVar(&focusID, "id", "", "焦点对象 ID")
	if directionFlag {
		command.Flags().StringVar(&direction, "direction", "both", "追踪方向：upstream、downstream 或 both")
	}
	return command
}

func (r *Root) auditCommand() *cobra.Command {
	command := &cobra.Command{Use: "audit", Short: "查看不可变的业务审计事件"}
	var limit int
	list := &cobra.Command{Use: "list", Short: "列出项目审计事件", RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		projectID, err := r.resolveProject(cmd, cfg, client)
		if err != nil {
			return err
		}
		var result []domain.AuditEvent
		if err := client.Dispatch(cmd.Context(), "audit.list", map[string]any{"project_id": projectID, "limit": limit}, &result); err != nil {
			return err
		}
		return r.writeOK("audit.list", result)
	}}
	list.Flags().IntVar(&limit, "limit", 50, "最多返回的事件数量")
	command.AddCommand(list)
	return command
}
