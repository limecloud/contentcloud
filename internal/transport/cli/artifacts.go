package cli

import (
	"encoding/base64"
	"encoding/json"
	"os"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/spf13/cobra"
)

func (r *Root) artifactCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "artifact", Short: "导出和下载批准快照成果"}

	var format, contentItemID string
	export := &cobra.Command{Use: "export <approved-snapshot-id>", Args: cobra.ExactArgs(1), Short: "导出一项已批准内容", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result deliverydomain.Artifact
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
			Artifact      deliverydomain.Artifact `json:"artifact"`
			ContentBase64 string                  `json:"content_base64"`
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

	var snapshotID, finalReviewID, packageID, manifestPath, projectID, output string
	jianying := &cobra.Command{Use: "jianying-export", Short: "导出已批准成片的确定性剪映归档", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		manifestBody, err := os.ReadFile(manifestPath)
		if err != nil {
			return err
		}
		var manifest deliverydomain.CompositionManifest
		if err := json.Unmarshal(manifestBody, &manifest); err != nil {
			return err
		}
		var result struct {
			FileName       string `json:"file_name"`
			ContentBase64  string `json:"content_base64"`
			ManifestDigest string `json:"manifest_digest"`
			ArchiveDigest  string `json:"archive_digest"`
			ByteSize       int    `json:"byte_size"`
		}
		if err := client.Dispatch(cmd.Context(), "jianying.export", map[string]any{"project_id": projectID, "approved_snapshot_id": snapshotID, "final_review_id": finalReviewID, "delivery_package_id": packageID, "manifest": manifest}, &result); err != nil {
			return err
		}
		body, err := base64.StdEncoding.DecodeString(result.ContentBase64)
		if err != nil {
			return err
		}
		if output == "" {
			output = result.FileName
		}
		if err := os.WriteFile(output, body, 0o600); err != nil {
			return err
		}
		return r.writeOK("jianying.export", map[string]any{"path": output, "manifest_digest": result.ManifestDigest, "archive_digest": result.ArchiveDigest, "byte_size": len(body)})
	}}
	jianying.Flags().StringVar(&projectID, "project", "", "项目 ID")
	jianying.Flags().StringVar(&snapshotID, "approved-snapshot-id", "", "批准快照 ID")
	jianying.Flags().StringVar(&finalReviewID, "final-review-id", "", "最终成片审核 ID")
	jianying.Flags().StringVar(&packageID, "delivery-package-id", "", "交付包 ID")
	jianying.Flags().StringVar(&manifestPath, "manifest", "", "CompositionManifest JSON 文件")
	jianying.Flags().StringVar(&output, "out", "", "ZIP 输出路径")
	for _, name := range []string{"project", "approved-snapshot-id", "final-review-id", "delivery-package-id", "manifest"} {
		_ = jianying.MarkFlagRequired(name)
	}
	cmd.AddCommand(jianying)
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
		var result deliverydomain.DeliveryPackage
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
		var result []deliverydomain.DeliveryPackage
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
		var result deliverydomain.DeliveryPackage
		if err := client.Dispatch(cmd.Context(), "delivery.show", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("delivery.show", result)
	}}

	cmd.AddCommand(create, list, show)
	return cmd
}
