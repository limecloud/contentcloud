package cli

import (
	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/contentprofile"
)

func (r *Root) contentProfileCommand() *cobra.Command {
	command := &cobra.Command{Use: "profile", Short: "管理可编译为现有 SOP 的内容生产 Profile"}
	list := &cobra.Command{Use: "list", Short: "列出内置内容生产 Profile", RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []contentprofile.Profile
		if err := client.Dispatch(cmd.Context(), "content_profile.list", map[string]any{}, &result); err != nil {
			return err
		}
		return r.writeOK("content_profile.list", result)
	}}
	install := &cobra.Command{Use: "install <profile-id>", Args: cobra.ExactArgs(1), Short: "幂等安装 Profile 为当前租户已发布 SOP", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result app.ContentProfileInstallResult
		if err := client.Dispatch(cmd.Context(), "content_profile.install", map[string]any{"profile_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("content_profile.install", result)
	}}
	command.AddCommand(list, install)
	return command
}
