package cli

import (
	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/integration/connector"
)

func (r *Root) connectorCommand() *cobra.Command {
	command := &cobra.Command{Use: "connector", Short: "同步 CMS、DAM、PIM、CRM 或 Agent SaaS 数据到 SourceRevision"}
	command.AddCommand(r.simpleUserReadCommand("adapters", "connector.adapter.list", "列出已配置 Connector Adapter", map[string]any{}))

	var projectID, connectorID, authorizationRef, region string
	bind := &cobra.Command{Use: "bind", Short: "创建只保存 SecretRef 的 Connector 绑定", RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result connector.Binding
		input := application.CreateConnectorBindingInput{ProjectID: projectID, ConnectorID: connectorID, AuthorizationRef: authorizationRef, Region: region}
		if err := client.Dispatch(cmd.Context(), "connector.binding.create", input, &result); err != nil {
			return err
		}
		return r.writeOK("connector.binding.create", result)
	}}
	bind.Flags().StringVar(&projectID, "project", "", "项目 ID")
	bind.Flags().StringVar(&connectorID, "connector", "", "Connector Adapter ID")
	bind.Flags().StringVar(&authorizationRef, "authorization-ref", "", "Secret Store 中的授权引用")
	bind.Flags().StringVar(&region, "region", "global", "数据区域")
	_ = bind.MarkFlagRequired("project")
	_ = bind.MarkFlagRequired("connector")
	_ = bind.MarkFlagRequired("authorization-ref")

	bindings := &cobra.Command{Use: "bindings", Short: "列出项目 Connector 绑定和当前游标", RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []connector.Binding
		if err := client.Dispatch(cmd.Context(), "connector.binding.list", map[string]any{"project_id": projectID}, &result); err != nil {
			return err
		}
		return r.writeOK("connector.binding.list", result)
	}}
	bindings.Flags().StringVar(&projectID, "project", "", "项目 ID")
	_ = bindings.MarkFlagRequired("project")

	var limit int
	syncCommand := &cobra.Command{Use: "sync <binding-id>", Args: cobra.ExactArgs(1), Short: "从当前 opaque cursor 增量拉取一页", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result connector.SyncReceipt
		if err := client.Dispatch(cmd.Context(), "connector.sync", map[string]any{"binding_id": args[0], "limit": limit}, &result); err != nil {
			return err
		}
		return r.writeOK("connector.sync", result)
	}}
	syncCommand.Flags().IntVar(&limit, "limit", 100, "单页上限，最大 1000")

	var bindingID string
	receipts := &cobra.Command{Use: "receipts", Short: "列出增量游标和物化摘要", RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []connector.SyncReceipt
		if err := client.Dispatch(cmd.Context(), "connector.receipt.list", map[string]any{"binding_id": bindingID}, &result); err != nil {
			return err
		}
		return r.writeOK("connector.receipt.list", result)
	}}
	receipts.Flags().StringVar(&bindingID, "binding", "", "可选 Connector 绑定 ID")
	command.AddCommand(bind, bindings, syncCommand, receipts)
	return command
}
