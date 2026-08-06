package cli

import (
	"encoding/base64"
	"os"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (r *Root) artifactCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "artifact", Short: "导出和下载批准快照成果"}

	var format, contentItemID string
	export := &cobra.Command{Use: "export <approved-snapshot-id>", Args: cobra.ExactArgs(1), Short: "导出一项已批准内容", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Artifact
		if err := client.Dispatch(cmd.Context(), "artifact.export", map[string]any{"snapshot_id": args[0], "content_item_id": contentItemID, "format": format}, &result); err != nil {
			return err
		}
		return r.writeOK("artifact.export", result)
	}}
	export.Flags().StringVar(&format, "format", "json", "导出格式：markdown、xlsx 或 json")
	export.Flags().StringVar(&contentItemID, "content-item", "", "批准快照包含多项内容时指定内容项 ID")

	var outputPath string
	download := &cobra.Command{Use: "download <artifact-id>", Args: cobra.ExactArgs(1), Short: "下载服务端保存的批准快照成果", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result struct {
			Artifact      domain.Artifact `json:"artifact"`
			ContentBase64 string          `json:"content_base64"`
		}
		if err := client.Dispatch(cmd.Context(), "artifact.download", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		data, err := base64.StdEncoding.DecodeString(result.ContentBase64)
		if err != nil {
			return err
		}
		path := outputPath
		if path == "" {
			path = result.Artifact.FileName
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
		return r.writeOK("artifact.download", map[string]any{"artifact_id": result.Artifact.ID, "path": path, "byte_size": len(data), "sha256": result.Artifact.SHA256})
	}}
	download.Flags().StringVar(&outputPath, "out", "", "输出文件路径")

	cmd.AddCommand(export, download)
	return cmd
}

func (r *Root) deliveryCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "delivery", Short: "管理不可变的批准快照交付包"}

	var contentItemID string
	create := &cobra.Command{Use: "create <approved-snapshot-id>", Args: cobra.ExactArgs(1), Short: "创建包含三种格式的交付包", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.DeliveryPackage
		if err := client.Dispatch(cmd.Context(), "delivery.create", map[string]any{"snapshot_id": args[0], "content_item_id": contentItemID}, &result); err != nil {
			return err
		}
		return r.writeOK("delivery.create", result)
	}}
	create.Flags().StringVar(&contentItemID, "content-item", "", "批准快照包含多项内容时指定内容项 ID")

	var projectID string
	list := &cobra.Command{Use: "list", Short: "列出不可变交付包", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.DeliveryPackage
		if err := client.Dispatch(cmd.Context(), "delivery.list", map[string]any{"project_id": projectID}, &result); err != nil {
			return err
		}
		return r.writeOK("delivery.list", result)
	}}
	list.Flags().StringVar(&projectID, "project", "", "项目 ID")
	_ = list.MarkFlagRequired("project")

	show := &cobra.Command{Use: "show <delivery-package-id>", Args: cobra.ExactArgs(1), Short: "显示交付包清单", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.DeliveryPackage
		if err := client.Dispatch(cmd.Context(), "delivery.show", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("delivery.show", result)
	}}

	cmd.AddCommand(create, list, show)
	return cmd
}
