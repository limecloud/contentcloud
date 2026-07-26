package cli

import (
	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (r *Root) lineageCommand() *cobra.Command {
	command := &cobra.Command{Use: "lineage", Short: "Trace project objects from sources through results"}
	command.AddCommand(r.lineageReadCommand("show", "lineage.show", "Show the bidirectional lineage graph", true))
	command.AddCommand(r.lineageReadCommand("impact", "lineage.impact", "Show downstream impact with recommended actions", false))
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
	command.Flags().StringVar(&focusType, "type", "", "focus object type")
	command.Flags().StringVar(&focusID, "id", "", "focus object ID")
	if directionFlag {
		command.Flags().StringVar(&direction, "direction", "both", "upstream, downstream, or both")
	}
	return command
}

func (r *Root) auditCommand() *cobra.Command {
	command := &cobra.Command{Use: "audit", Short: "Inspect immutable business audit events"}
	var limit int
	list := &cobra.Command{Use: "list", Short: "List project audit events", RunE: func(cmd *cobra.Command, _ []string) error {
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
	list.Flags().IntVar(&limit, "limit", 50, "maximum event count")
	command.AddCommand(list)
	return command
}
