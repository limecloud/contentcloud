package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
)

func (r *Root) authCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "管理与设备凭据相互独立的用户登录凭据"}
	var noWait bool
	var deviceCode string
	login := &cobra.Command{Use: "login", Short: "发起或完成需要浏览器确认的设备登录", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		server := r.resolveServer(cfg)
		client := apiclient.New(server, "")
		if noWait {
			var result app.StartDeviceLoginResult
			if err := client.Dispatch(cmd.Context(), "auth.login.start", map[string]any{}, &result); err != nil {
				return err
			}
			cfg.ServerURL = server
			if err := localconfig.Save(cfg); err != nil {
				return err
			}
			return r.writeOK("auth.login", result)
		}
		if deviceCode == "" {
			return domain.Invalid("DEVICE_CODE_REQUIRED", "使用 --no-wait 发起登录，或用 --device-code 完成登录")
		}
		var result app.CompleteDeviceLoginResult
		if err := client.Dispatch(cmd.Context(), "auth.login.complete", map[string]any{"device_code": deviceCode}, &result); err != nil {
			return err
		}
		if err := localconfig.SaveUserToken(server, result.AccessToken); err != nil {
			return &domain.Error{Type: "credential", Subtype: "secure_store", Code: "CREDENTIAL_STORE_FAILED", Message: err.Error(), ExitCode: 3}
		}
		result.AccessToken = ""
		cfg.ServerURL = server
		if err := localconfig.Save(cfg); err != nil {
			return err
		}
		return r.writeOK("auth.login", map[string]any{"authenticated": true, "tenant": result.Tenant, "expires_at": result.ExpiresAt, "credential_store": credentialProvider()})
	}}
	login.Flags().BoolVar(&noWait, "no-wait", false, "发起登录后立即返回，等待在浏览器中确认")
	login.Flags().StringVar(&deviceCode, "device-code", "", "完成此前已在浏览器批准的设备登录")
	status := &cobra.Command{Use: "status", Short: "显示用户登录状态，但不暴露凭据", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		server := r.resolveServer(cfg)
		token, err := localconfig.UserToken(server)
		if err != nil {
			return r.writeOK("auth.status", map[string]any{"authenticated": false, "server_url": server, "credential_store": credentialProvider()})
		}
		var result map[string]any
		if err := apiclient.New(server, token).Dispatch(cmd.Context(), "auth.status", map[string]any{}, &result); err != nil {
			return err
		}
		return r.writeOK("auth.status", result)
	}}
	logout := &cobra.Command{Use: "logout", Short: "撤销并移除用户命令行凭据", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		if err := client.Dispatch(cmd.Context(), "auth.logout", map[string]any{}, nil); err != nil {
			return err
		}
		if err := localconfig.DeleteUserToken(r.resolveServer(cfg)); err != nil {
			return err
		}
		return r.writeOK("auth.logout", map[string]any{"logged_out": true})
	}}
	cmd.AddCommand(login, status, logout)
	return cmd
}

func (r *Root) tenantCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "tenant", Short: "查看当前可用的租户上下文"}
	cmd.AddCommand(r.simpleListCommand("list", "tenant.list", "列出当前用户可访问的租户", func(_ localconfig.Config) map[string]any { return map[string]any{} }))
	var dryRun bool
	switchCmd := &cobra.Command{Use: "switch <tenant-id>", Args: cobra.ExactArgs(1), Short: "切换到另一个已授权租户并更新命令行凭据", RunE: func(cmd *cobra.Command, args []string) error {
		if dryRun {
			return r.writeOK("tenant.switch", map[string]any{"dry_run": true, "tenant_id": args[0]})
		}
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result app.SwitchCLITenantResult
		if err := client.Dispatch(cmd.Context(), "tenant.switch", map[string]any{"tenant_id": args[0]}, &result); err != nil {
			return err
		}
		server := r.resolveServer(cfg)
		if err := localconfig.SaveUserToken(server, result.AccessToken); err != nil {
			return &domain.Error{Type: "credential", Subtype: "secure_store", Code: "CREDENTIAL_STORE_FAILED", Message: err.Error(), ExitCode: 3}
		}
		result.AccessToken = ""
		return r.writeOK("tenant.switch", map[string]any{"tenant": result.Tenant, "expires_at": result.ExpiresAt})
	}}
	switchCmd.Flags().BoolVar(&dryRun, "dry-run", false, "只验证，不更新命令行凭据")
	cmd.AddCommand(switchCmd)
	return cmd
}

func (r *Root) teamCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "team", Short: "管理租户成员、固定角色和邀请"}
	cmd.AddCommand(r.simpleListCommand("members", "membership.list", "列出租户成员", func(_ localconfig.Config) map[string]any { return map[string]any{} }))
	invites := &cobra.Command{Use: "invites", Short: "列出租户邀请", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.MembershipInvite
		if err := client.Dispatch(cmd.Context(), "membership.invite.list", map[string]any{}, &result); err != nil {
			return err
		}
		return r.writeOK("membership.invite.list", result)
	}}
	var inviteRole string
	var inviteDryRun bool
	invite := &cobra.Command{Use: "invite <email>", Args: cobra.ExactArgs(1), Short: "创建有效期为 72 小时的租户邀请", RunE: func(cmd *cobra.Command, args []string) error {
		if inviteDryRun {
			return r.writeOK("membership.invite.create", map[string]any{"dry_run": true, "email": args[0], "role": inviteRole})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.MembershipInvite
		if err := client.Dispatch(cmd.Context(), "membership.invite.create", map[string]any{"email": args[0], "role": inviteRole}, &result); err != nil {
			return err
		}
		return r.writeOK("membership.invite.create", result)
	}}
	invite.Flags().StringVar(&inviteRole, "role", "viewer", "租户角色：tenant_admin、project_manager、strategist、editor、reviewer 或 viewer")
	invite.Flags().BoolVar(&inviteDryRun, "dry-run", false, "只验证，不创建邀请")
	var acceptDryRun bool
	accept := &cobra.Command{Use: "accept <invite-token>", Args: cobra.ExactArgs(1), Short: "接受与当前登录邮箱绑定的邀请", RunE: func(cmd *cobra.Command, args []string) error {
		if acceptDryRun {
			return r.writeOK("membership.invite.accept", map[string]any{"dry_run": true})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Membership
		if err := client.Dispatch(cmd.Context(), "membership.invite.accept", map[string]any{"token": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("membership.invite.accept", result)
	}}
	accept.Flags().BoolVar(&acceptDryRun, "dry-run", false, "只验证，不接受邀请")
	var revokeInviteYes, revokeInviteDryRun bool
	revokeInvite := &cobra.Command{Use: "revoke-invite <invite-id>", Args: cobra.ExactArgs(1), Short: "撤销一条待接受的租户邀请", RunE: func(cmd *cobra.Command, args []string) error {
		if revokeInviteDryRun {
			return r.writeOK("membership.invite.revoke", map[string]any{"dry_run": true, "id": args[0]})
		}
		if !revokeInviteYes {
			return confirmationRequired("撤销邀请后，原邀请令牌将立即失效")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.MembershipInvite
		if err := client.Dispatch(cmd.Context(), "membership.invite.revoke", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("membership.invite.revoke", result)
	}}
	revokeInvite.Flags().BoolVar(&revokeInviteYes, "yes", false, "确认执行这项高风险写入")
	revokeInvite.Flags().BoolVar(&revokeInviteDryRun, "dry-run", false, "只验证，不撤销邀请")
	var roleDryRun bool
	setRole := &cobra.Command{Use: "set-role <user-id> <role>", Args: cobra.ExactArgs(2), Short: "为成员分配一个固定租户角色", RunE: func(cmd *cobra.Command, args []string) error {
		if roleDryRun {
			return r.writeOK("membership.update", map[string]any{"dry_run": true, "user_id": args[0], "role": args[1]})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Membership
		if err := client.Dispatch(cmd.Context(), "membership.update", map[string]any{"user_id": args[0], "role": args[1]}, &result); err != nil {
			return err
		}
		return r.writeOK("membership.update", result)
	}}
	setRole.Flags().BoolVar(&roleDryRun, "dry-run", false, "只验证，不变更角色")
	var revokeYes, revokeDryRun bool
	revoke := &cobra.Command{Use: "revoke <user-id>", Args: cobra.ExactArgs(1), Short: "撤销成员资格及其当前租户会话", RunE: func(cmd *cobra.Command, args []string) error {
		if revokeDryRun {
			return r.writeOK("membership.revoke", map[string]any{"dry_run": true, "user_id": args[0]})
		}
		if !revokeYes {
			return confirmationRequired("撤销成员会立即使其当前租户会话失效")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Membership
		if err := client.Dispatch(cmd.Context(), "membership.revoke", map[string]any{"user_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("membership.revoke", result)
	}}
	revoke.Flags().BoolVar(&revokeYes, "yes", false, "确认执行这项高风险写入")
	revoke.Flags().BoolVar(&revokeDryRun, "dry-run", false, "只验证，不撤销成员资格")
	cmd.AddCommand(invites, invite, accept, revokeInvite, setRole, revoke)
	return cmd
}

func (r *Root) fullProjectCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "查找、解析并查看项目"}
	cmd.AddCommand(r.simpleListCommand("list", "project.list", "列出当前用户可访问的项目", func(_ localconfig.Config) map[string]any { return map[string]any{} }))
	show := &cobra.Command{Use: "show [project-id]", Args: cobra.MaximumNArgs(1), Short: "显示一个项目", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		id := ""
		if len(args) == 1 {
			id = args[0]
		} else {
			id, err = r.resolveProject(cmd, cfg, client)
			if err != nil {
				return err
			}
		}
		var result domain.Project
		if err := client.Dispatch(cmd.Context(), "project.show", map[string]any{"project_id": id}, &result); err != nil {
			return err
		}
		return r.writeOK("project.show", result)
	}}
	resolve := &cobra.Command{Use: "resolve <name-or-slug>", Args: cobra.ExactArgs(1), Short: "将项目名称或短标识解析为稳定 ID", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		projects := []domain.Project{}
		if err := client.Dispatch(cmd.Context(), "project.list", map[string]any{}, &projects); err != nil {
			return err
		}
		query := strings.ToLower(strings.TrimSpace(args[0]))
		matches := []domain.Project{}
		for _, project := range projects {
			if project.ID == args[0] || strings.ToLower(project.Slug) == query || strings.ToLower(project.BrandName+" "+project.ProductName) == query {
				matches = append(matches, project)
			}
		}
		if len(matches) != 1 {
			return domain.Invalid("PROJECT_RESOLUTION_AMBIGUOUS", "项目名称未唯一匹配，请使用 project list 后传入 ID")
		}
		return r.writeOK("project.resolve", map[string]any{"project_id": matches[0].ID, "slug": matches[0].Slug, "brand_name": matches[0].BrandName, "product_name": matches[0].ProductName})
	}}
	var brand, product, channel, objective, owner, reviewer, approver, templateID string
	var createDryRun bool
	create := &cobra.Command{Use: "create", Short: "创建一个聚焦单一产品的项目", RunE: func(cmd *cobra.Command, args []string) error {
		input := app.CreateProjectInput{TemplateID: templateID, BrandName: brand, ProductName: product, Channel: channel, StageObjective: objective, OwnerName: owner, ReviewerName: reviewer, ClientApprover: approver}
		if createDryRun {
			return r.writeOK("project.create", map[string]any{"dry_run": true, "input": input})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Project
		if err := client.Dispatch(cmd.Context(), "project.create", input, &result); err != nil {
			return err
		}
		return r.writeOK("project.create", result)
	}}
	create.Flags().StringVar(&brand, "brand", "", "品牌名称")
	create.Flags().StringVar(&product, "product", "", "项目聚焦的产品名称")
	create.Flags().StringVar(&channel, "channel", "", "目标渠道")
	create.Flags().StringVar(&objective, "objective", "", "当前阶段目标")
	create.Flags().StringVar(&owner, "owner", "", "项目负责人显示名称")
	create.Flags().StringVar(&reviewer, "reviewer", "", "内部审核人显示名称")
	create.Flags().StringVar(&approver, "client-approver", "", "客户审批人显示名称")
	create.Flags().StringVar(&templateID, "template", "", "租户项目模板 ID")
	create.Flags().BoolVar(&createDryRun, "dry-run", false, "只验证，不创建项目")
	_ = create.MarkFlagRequired("brand")
	_ = create.MarkFlagRequired("product")
	var lifecycleYes, lifecycleDryRun bool
	lifecycle := func(action string) *cobra.Command {
		var rowVersion int
		short := "归档项目，并使用乐观锁避免并发覆盖"
		if action == "restore" {
			short = "恢复项目，并使用乐观锁避免并发覆盖"
		}
		command := &cobra.Command{Use: action + " <project-id>", Args: cobra.ExactArgs(1), Short: short, RunE: func(cmd *cobra.Command, args []string) error {
			if lifecycleDryRun {
				return r.writeOK("project."+action, map[string]any{"dry_run": true, "project_id": args[0], "row_version": rowVersion})
			}
			if !lifecycleYes {
				return confirmationRequired("项目归档或恢复会改变整个项目的写入状态")
			}
			_, client, _, err := r.userClient()
			if err != nil {
				return err
			}
			var result domain.Project
			if err := client.Dispatch(cmd.Context(), "project."+action, map[string]any{"project_id": args[0], "row_version": rowVersion}, &result); err != nil {
				return err
			}
			return r.writeOK("project."+action, result)
		}}
		command.Flags().IntVar(&rowVersion, "row-version", 0, "必填，项目当前的行版本号")
		command.Flags().BoolVar(&lifecycleYes, "yes", false, "确认执行这项高风险写入")
		command.Flags().BoolVar(&lifecycleDryRun, "dry-run", false, "只验证，不改变项目状态")
		_ = command.MarkFlagRequired("row-version")
		return command
	}
	templates := &cobra.Command{Use: "templates", Short: "列出已脱敏的租户项目模板", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.ProjectTemplate
		if err := client.Dispatch(cmd.Context(), "project_template.list", map[string]any{}, &result); err != nil {
			return err
		}
		return r.writeOK("project_template.list", result)
	}}
	var templateName, templateChannel, templateObjective string
	var templateDryRun bool
	createTemplate := &cobra.Command{Use: "create", Short: "创建不含客户事实和素材的脱敏项目模板", RunE: func(cmd *cobra.Command, args []string) error {
		input := app.CreateProjectTemplateInput{Name: templateName, Channel: templateChannel, StageObjective: templateObjective}
		if templateDryRun {
			return r.writeOK("project_template.create", map[string]any{"dry_run": true, "input": input})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ProjectTemplate
		if err := client.Dispatch(cmd.Context(), "project_template.create", input, &result); err != nil {
			return err
		}
		return r.writeOK("project_template.create", result)
	}}
	createTemplate.Flags().StringVar(&templateName, "name", "", "模板名称")
	createTemplate.Flags().StringVar(&templateChannel, "channel", "douyin", "默认目标渠道")
	createTemplate.Flags().StringVar(&templateObjective, "objective", "", "默认阶段目标")
	createTemplate.Flags().BoolVar(&templateDryRun, "dry-run", false, "只验证，不创建模板")
	_ = createTemplate.MarkFlagRequired("name")
	templates.AddCommand(createTemplate)
	var updateRowVersion int
	var updateBrand, updateProduct, updateChannel, updateObjective, updateOwner, updateReviewer, updateApprover string
	var updateDryRun bool
	update := &cobra.Command{Use: "update <project-id>", Args: cobra.ExactArgs(1), Short: "更新项目资料，并使用乐观锁避免并发覆盖", RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]any{"project_id": args[0], "row_version": updateRowVersion}
		for name, value := range map[string]string{"brand_name": updateBrand, "product_name": updateProduct, "channel": updateChannel, "stage_objective": updateObjective, "owner_name": updateOwner, "reviewer_name": updateReviewer, "client_approver": updateApprover} {
			flagName := map[string]string{"brand_name": "brand", "product_name": "product", "channel": "channel", "stage_objective": "objective", "owner_name": "owner", "reviewer_name": "reviewer", "client_approver": "client-approver"}[name]
			if cmd.Flags().Changed(flagName) {
				params[name] = value
			}
		}
		if updateDryRun {
			params["dry_run"] = true
			return r.writeOK("project.update", params)
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Project
		if err := client.Dispatch(cmd.Context(), "project.update", params, &result); err != nil {
			return err
		}
		return r.writeOK("project.update", result)
	}}
	update.Flags().IntVar(&updateRowVersion, "row-version", 0, "必填，项目当前的行版本号")
	update.Flags().StringVar(&updateBrand, "brand", "", "品牌名称")
	update.Flags().StringVar(&updateProduct, "product", "", "项目聚焦的产品名称")
	update.Flags().StringVar(&updateChannel, "channel", "", "目标渠道")
	update.Flags().StringVar(&updateObjective, "objective", "", "阶段目标")
	update.Flags().StringVar(&updateOwner, "owner", "", "项目负责人显示名称")
	update.Flags().StringVar(&updateReviewer, "reviewer", "", "内部审核人显示名称")
	update.Flags().StringVar(&updateApprover, "client-approver", "", "客户审批人显示名称")
	update.Flags().BoolVar(&updateDryRun, "dry-run", false, "只验证，不修改项目")
	_ = update.MarkFlagRequired("row-version")
	cmd.AddCommand(show, resolve, create, update, lifecycle("archive"), lifecycle("restore"), templates)
	return cmd
}

func (r *Root) deviceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "device", Short: "查看和撤销已连接的创作运行环境"}
	cmd.AddCommand(r.projectListCommand("list", "device.list", "列出项目已授权的设备", "project_id"))
	var connectDryRun bool
	connectCreate := &cobra.Command{Use: "connect-create [project-id]", Args: cobra.MaximumNArgs(1), Short: "创建有效期为 10 分钟的项目连接会话", RunE: func(cmd *cobra.Command, args []string) error {
		projectID := ""
		if len(args) == 1 {
			projectID = args[0]
		} else if connectDryRun && r.projectID != "" {
			projectID = r.projectID
		} else {
			cfg, client, _, err := r.userClient()
			if err != nil {
				return err
			}
			projectID, err = r.resolveProject(cmd, cfg, client)
			if err != nil {
				return err
			}
		}
		if connectDryRun {
			return r.writeOK("device.connect_session.create", map[string]any{"dry_run": true, "project_id": projectID})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ConnectSession
		if err := client.Dispatch(cmd.Context(), "device.connect_session.create", map[string]any{"project_id": projectID}, &result); err != nil {
			return err
		}
		return r.writeOK("device.connect_session.create", result)
	}}
	connectCreate.Flags().BoolVar(&connectDryRun, "dry-run", false, "只验证，不创建连接会话")
	connectShow := &cobra.Command{Use: "connect-show <session-id>", Args: cobra.ExactArgs(1), Short: "显示项目连接会话", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ConnectSession
		if err := client.Dispatch(cmd.Context(), "device.connect_session.show", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("device.connect_session.show", result)
	}}
	var cancelConnectYes, cancelConnectDryRun bool
	connectCancel := &cobra.Command{Use: "connect-cancel <session-id>", Args: cobra.ExactArgs(1), Short: "取消等待中的项目连接会话", RunE: func(cmd *cobra.Command, args []string) error {
		if cancelConnectDryRun {
			return r.writeOK("device.connect_session.cancel", map[string]any{"dry_run": true, "id": args[0]})
		}
		if !cancelConnectYes {
			return confirmationRequired("取消连接会话会立即使当前浏览器设备授权失效")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ConnectSession
		if err := client.Dispatch(cmd.Context(), "device.connect_session.cancel", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("device.connect_session.cancel", result)
	}}
	connectCancel.Flags().BoolVar(&cancelConnectYes, "yes", false, "确认执行这项高风险写入")
	connectCancel.Flags().BoolVar(&cancelConnectDryRun, "dry-run", false, "只验证，不取消连接会话")
	show := &cobra.Command{Use: "show <device-id>", Args: cobra.ExactArgs(1), Short: "显示一个已连接的创作运行环境", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Device
		if err := client.Dispatch(cmd.Context(), "device.show", map[string]any{"device_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("device.show", result)
	}}
	cmd.AddCommand(show, connectCreate, connectShow, connectCancel, r.deviceGrantCommand("attach", "允许设备访问当前项目"), r.deviceGrantCommand("detach", "移除设备对当前项目的访问权限"))
	var yes, dryRun bool
	revoke := &cobra.Command{Use: "revoke <device-id>", Args: cobra.ExactArgs(1), Short: "立即撤销设备", RunE: func(cmd *cobra.Command, args []string) error {
		if dryRun {
			return r.writeOK("device.revoke", map[string]any{"dry_run": true, "device_id": args[0], "would_revoke": true})
		}
		if !yes {
			return confirmationRequired("设备撤销会立即停止该设备领取新任务")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Device
		if err := client.Dispatch(cmd.Context(), "device.revoke", map[string]any{"device_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("device.revoke", result)
	}}
	revoke.Flags().BoolVar(&yes, "yes", false, "确认执行这项高风险写入")
	revoke.Flags().BoolVar(&dryRun, "dry-run", false, "只验证，不改变服务端状态")
	cmd.AddCommand(revoke)
	return cmd
}

func (r *Root) deviceGrantCommand(action, short string) *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{Use: action + " <device-id>", Args: cobra.ExactArgs(1), Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		projectID, err := r.resolveProject(cmd, cfg, client)
		if err != nil {
			return err
		}
		if dryRun {
			return r.writeOK("device."+action, map[string]any{"dry_run": true, "device_id": args[0], "project_id": projectID})
		}
		if action == "detach" && !yes {
			return confirmationRequired("移除项目设备授权会阻止该设备继续领取此项目任务")
		}
		var result domain.Device
		if err := client.Dispatch(cmd.Context(), "device."+action, map[string]any{"device_id": args[0], "project_id": projectID}, &result); err != nil {
			return err
		}
		return r.writeOK("device."+action, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "只验证，不改变服务端状态")
	if action == "detach" {
		cmd.Flags().BoolVar(&yes, "yes", false, "确认执行这项高风险写入")
	}
	return cmd
}

func (r *Root) sourceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "source", Short: "上传和查看受治理的来源文件"}
	cmd.AddCommand(r.projectListCommand("list", "source.list", "列出项目来源", "project_id"))
	var name, sourceType, mimeType string
	var uploadDryRun bool
	upload := &cobra.Command{Use: "upload <file>", Args: cobra.ExactArgs(1), Short: "上传一个受支持且不可变的来源版本", RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		if len(data) > 100*1024*1024 {
			return domain.Invalid("SOURCE_SIZE_INVALID", "文件超过 100MB")
		}
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		projectID, err := localconfig.ResolveProject(r.projectID, cfg)
		if err != nil {
			return domain.Invalid("PROJECT_CONTEXT_REQUIRED", "未解析到唯一项目上下文")
		}
		if mimeType == "" {
			mimeType = mimeForFile(args[0])
		}
		params := map[string]any{"project_id": projectID, "name": defaultValue(name, filepath.Base(args[0])), "source_type": defaultValue(sourceType, "brand_manual"), "file_name": filepath.Base(args[0]), "mime": mimeType, "content_base64": base64.StdEncoding.EncodeToString(data)}
		if uploadDryRun {
			return r.writeOK("source.upload", map[string]any{"dry_run": true, "project_id": projectID, "file_name": filepath.Base(args[0]), "mime": mimeType, "byte_size": len(data)})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.SourceRevision
		if err := client.Dispatch(cmd.Context(), "source.upload", params, &result); err != nil {
			return err
		}
		return r.writeOK("source.upload", result)
	}}
	upload.Flags().StringVar(&name, "name", "", "逻辑来源名称")
	upload.Flags().StringVar(&sourceType, "type", "brand_manual", "来源类型")
	upload.Flags().StringVar(&mimeType, "mime", "", "声明的 MIME 类型；默认根据扩展名推断")
	upload.Flags().BoolVar(&uploadDryRun, "dry-run", false, "只验证，不上传文件或改变服务端状态")
	status := &cobra.Command{Use: "status <revision-id>", Args: cobra.ExactArgs(1), Short: "显示不可变来源版本的处理状态", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.SourceRevision
		if err := client.Dispatch(cmd.Context(), "source.status", map[string]any{"revision_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("source.status", result)
	}}
	revisions := r.idReadCommand("revisions <source-id>", "source.revisions", "列出一个逻辑来源的全部不可变版本", "source_id")
	impact := r.idReadCommand("impact <source-id>", "source.impact", "显示受该来源影响的治理知识", "source_id")
	var reviseMIME string
	var reviseDryRun bool
	revise := &cobra.Command{Use: "revise <source-id> <file>", Args: cobra.ExactArgs(2), Short: "为现有逻辑来源上传新的不可变版本", RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		if len(data) == 0 || len(data) > 100*1024*1024 {
			return domain.Invalid("SOURCE_SIZE_INVALID", "文件必须在 1B 到 100MB 之间")
		}
		if reviseMIME == "" {
			reviseMIME = mimeForFile(args[1])
		}
		if reviseDryRun {
			return r.writeOK("source.revise", map[string]any{"dry_run": true, "source_id": args[0], "file_name": filepath.Base(args[1]), "mime": reviseMIME, "byte_size": len(data)})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.SourceRevision
		params := map[string]any{"source_id": args[0], "file_name": filepath.Base(args[1]), "mime": reviseMIME, "content_base64": base64.StdEncoding.EncodeToString(data)}
		if err := client.Dispatch(cmd.Context(), "source.revise", params, &result); err != nil {
			return err
		}
		return r.writeOK("source.revise", result)
	}}
	revise.Flags().StringVar(&reviseMIME, "mime", "", "声明的 MIME 类型；默认根据扩展名推断")
	revise.Flags().BoolVar(&reviseDryRun, "dry-run", false, "只验证，不上传文件或改变服务端状态")
	var evidenceDryRun bool
	evidenceReview := &cobra.Command{Use: "evidence-review <evidence-id> <accept|reject>", Args: cobra.ExactArgs(2), Short: "记录对一个已提取证据片段的人工决定", RunE: func(cmd *cobra.Command, args []string) error {
		if args[1] != "accept" && args[1] != "reject" {
			return domain.Invalid("EVIDENCE_DECISION_INVALID", "证据复核只允许 accept 或 reject")
		}
		if evidenceDryRun {
			return r.writeOK("evidence.review", map[string]any{"dry_run": true, "evidence_id": args[0], "decision": args[1]})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.EvidenceSpan
		if err := client.Dispatch(cmd.Context(), "evidence.review", map[string]any{"evidence_id": args[0], "decision": args[1]}, &result); err != nil {
			return err
		}
		return r.writeOK("evidence.review", result)
	}}
	evidenceReview.Flags().BoolVar(&evidenceDryRun, "dry-run", false, "只验证，不改变服务端状态")
	cmd.AddCommand(upload, status, revisions, impact, revise, evidenceReview)
	return cmd
}

func (r *Root) assetCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "asset", Short: "治理生成素材及其使用权利"}
	cmd.AddCommand(r.projectListCommand("list", "asset.list", "列出项目中受治理的素材", "project_id"))

	var name, assetType, sourceRevisionID, usageMode string
	var createDryRun bool
	create := &cobra.Command{Use: "create", Short: "将一个来源版本登记为受治理素材", RunE: func(command *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		projectID, err := localconfig.ResolveProject(r.projectID, cfg)
		if err != nil {
			return domain.Invalid("PROJECT_CONTEXT_REQUIRED", "未解析到唯一项目上下文")
		}
		input := app.CreateAssetInput{ProjectID: projectID, Name: name, AssetType: assetType, SourceRevisionID: sourceRevisionID, UsageMode: usageMode}
		if createDryRun {
			return r.writeOK("asset.create", map[string]any{"dry_run": true, "input": input})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.Asset
		if err := client.Dispatch(command.Context(), "asset.create", input, &result); err != nil {
			return err
		}
		return r.writeOK("asset.create", result)
	}}
	create.Flags().StringVar(&name, "name", "", "素材显示名称")
	create.Flags().StringVar(&assetType, "type", "product_image", "素材类型：product_image、brand_mark、packaging、person、location、audio 或 other")
	create.Flags().StringVar(&sourceRevisionID, "source-revision", "", "包含不可变素材文件且状态为 ready 的来源版本")
	create.Flags().StringVar(&usageMode, "usage", "analysis_only", "使用方式：analysis_only、generation_reference 或 owned")
	create.Flags().BoolVar(&createDryRun, "dry-run", false, "只在本地验证，不改变服务端状态")
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("source-revision")

	rights := &cobra.Command{Use: "rights <asset-id>", Args: cobra.ExactArgs(1), Short: "列出素材的权利记录", RunE: func(command *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.RightsRecord
		if err := client.Dispatch(command.Context(), "rights.list", map[string]any{"asset_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("rights.list", result)
	}}

	var holder, rightsType, proofRevision, validFrom, validUntil string
	var territories, channels, restrictions []string
	var rightsDryRun bool
	rightsCreate := &cobra.Command{Use: "rights-create <asset-id>", Args: cobra.ExactArgs(1), Short: "创建一条待审核的权利记录", RunE: func(command *cobra.Command, args []string) error {
		from, err := parseOptionalRFC3339(validFrom, "--valid-from")
		if err != nil {
			return err
		}
		until, err := parseOptionalRFC3339(validUntil, "--valid-until")
		if err != nil {
			return err
		}
		input := app.CreateRightsRecordInput{AssetID: args[0], RightsHolder: holder, RightsType: rightsType, Territories: territories, Channels: channels, ValidFrom: from, ValidUntil: until, ProofSourceRevisionID: proofRevision, Restrictions: restrictions}
		if rightsDryRun {
			return r.writeOK("rights.create", map[string]any{"dry_run": true, "input": input})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.RightsRecord
		if err := client.Dispatch(command.Context(), "rights.create", input, &result); err != nil {
			return err
		}
		return r.writeOK("rights.create", result)
	}}
	rightsCreate.Flags().StringVar(&holder, "holder", "", "权利持有人")
	rightsCreate.Flags().StringVar(&rightsType, "type", "owned", "权利类型：owned、licensed_generation、licensed_edit 或 public_domain")
	rightsCreate.Flags().StringSliceVar(&territories, "territory", []string{"CN"}, "允许使用的地区；可重复传入或用逗号分隔")
	rightsCreate.Flags().StringSliceVar(&channels, "channel", nil, "允许使用的渠道；可重复传入或用逗号分隔")
	rightsCreate.Flags().StringVar(&proofRevision, "proof-source-revision", "", "包含权利证明且状态为 ready 的来源版本")
	rightsCreate.Flags().StringVar(&validFrom, "valid-from", "", "可选的 RFC3339 生效时间")
	rightsCreate.Flags().StringVar(&validUntil, "valid-until", "", "可选的 RFC3339 失效时间")
	rightsCreate.Flags().StringSliceVar(&restrictions, "restriction", nil, "使用限制；可重复传入或用逗号分隔")
	rightsCreate.Flags().BoolVar(&rightsDryRun, "dry-run", false, "只在本地验证，不改变服务端状态")
	_ = rightsCreate.MarkFlagRequired("holder")
	_ = rightsCreate.MarkFlagRequired("channel")
	_ = rightsCreate.MarkFlagRequired("proof-source-revision")

	var reviewDryRun bool
	review := &cobra.Command{Use: "rights-review <rights-id> <approve|reject>", Args: cobra.ExactArgs(2), Short: "记录对一条权利记录的审核决定", RunE: func(command *cobra.Command, args []string) error {
		if args[1] != "approve" && args[1] != "reject" {
			return domain.Invalid("RIGHTS_DECISION_INVALID", "权利审核只允许 approve 或 reject")
		}
		if reviewDryRun {
			return r.writeOK("rights.review", map[string]any{"dry_run": true, "id": args[0], "decision": args[1]})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.RightsRecord
		if err := client.Dispatch(command.Context(), "rights.review", map[string]any{"id": args[0], "decision": args[1]}, &result); err != nil {
			return err
		}
		return r.writeOK("rights.review", result)
	}}
	review.Flags().BoolVar(&reviewDryRun, "dry-run", false, "只验证，不改变服务端状态")
	cmd.AddCommand(create, rights, rightsCreate, review)
	return cmd
}

func parseOptionalRFC3339(value, flag string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, domain.Invalid("TIME_INVALID", flag+" 必须是 RFC3339 时间")
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func (r *Root) knowledgeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "knowledge", Short: "查看并审核受治理的知识对象"}
	cmd.AddCommand(r.projectListCommand("list", "knowledge.list", "列出项目知识对象", "project_id"))
	var sourceRevisionIDs []string
	var outputCount int
	var idempotencyKey string
	var extractDryRun bool
	extract := &cobra.Command{Use: "extract", Short: "创建一次基于本地证据的知识提取任务", RunE: func(command *cobra.Command, args []string) error {
		input := app.CreateKnowledgeExtractionRunInput{SourceRevisionIDs: sourceRevisionIDs, IdempotencyKey: idempotencyKey, OutputCount: outputCount}
		if extractDryRun {
			cfg, err := localconfig.Load()
			if err != nil {
				return err
			}
			projectID, err := localconfig.ResolveProject(r.projectID, cfg)
			if err != nil {
				return domain.Invalid("PROJECT_CONTEXT_REQUIRED", "请用 --project 或本地项目上下文指定项目")
			}
			input.ProjectID = projectID
			return r.writeOK("knowledge.extract", map[string]any{"dry_run": true, "input": input})
		}
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		projectID, err := r.resolveProject(command, cfg, client)
		if err != nil {
			return err
		}
		input.ProjectID = projectID
		var result domain.RuntimeRun
		if err := client.Dispatch(command.Context(), "knowledge.extract", input, &result); err != nil {
			return err
		}
		return r.writeOK("knowledge.extract", result)
	}}
	extract.Flags().StringSliceVar(&sourceRevisionIDs, "source-revision", nil, "状态为 ready 的来源版本 ID；可重复传入或用逗号分隔")
	extract.Flags().IntVar(&outputCount, "count", 20, "知识候选项最大数量，范围为 1 到 20")
	extract.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "用于安全重试的稳定幂等键")
	extract.Flags().BoolVar(&extractDryRun, "dry-run", false, "只验证，不创建服务端任务")
	_ = extract.MarkFlagRequired("source-revision")
	var dryRun bool
	var reason string
	review := &cobra.Command{Use: "review <id> <approve|reject>", Args: cobra.ExactArgs(2), Short: "记录一次知识对象人工审核决定", RunE: func(cmd *cobra.Command, args []string) error {
		if dryRun {
			return r.writeOK("knowledge.review", map[string]any{"dry_run": true, "id": args[0], "decision": args[1], "reason": reason})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var current domain.KnowledgeObject
		if err := client.Dispatch(cmd.Context(), "knowledge.show", map[string]any{"id": args[0]}, &current); err != nil {
			return err
		}
		var result struct {
			Object   domain.KnowledgeObject   `json:"object"`
			Decision domain.KnowledgeDecision `json:"decision"`
		}
		if err := client.Dispatch(cmd.Context(), "knowledge.review", map[string]any{"id": args[0], "expected_version": current.Version, "expected_digest": current.Digest, "decision": args[1], "reason": reason}, &result); err != nil {
			return err
		}
		return r.writeOK("knowledge.review", result)
	}}
	review.Flags().BoolVar(&dryRun, "dry-run", false, "只验证，不改变服务端状态")
	review.Flags().StringVar(&reason, "reason", "通过命令行审核知识", "审核理由")
	cmd.AddCommand(r.idReadCommand("show <knowledge-id>", "knowledge.show", "显示一个知识对象", "id"), extract, review)
	return cmd
}

func (r *Root) runCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "run", Short: "查看和取消自动化任务"}
	cmd.AddCommand(r.projectListCommand("list", "run.list", "列出项目任务", "project_id"), r.idReadCommand("show <run-id>", "run.show", "显示一个任务", "id"))
	var yes, dryRun bool
	cancel := &cobra.Command{Use: "cancel <run-id>", Args: cobra.ExactArgs(1), Short: "请求取消排队中或运行中的任务", RunE: func(cmd *cobra.Command, args []string) error {
		if dryRun {
			return r.writeOK("run.cancel", map[string]any{"dry_run": true, "run_id": args[0]})
		}
		if !yes {
			return confirmationRequired("任务取消可能终止正在运行的本地 Agent 进程")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.RuntimeRun
		if err := client.Dispatch(cmd.Context(), "run.cancel", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("run.cancel", result)
	}}
	cancel.Flags().BoolVar(&yes, "yes", false, "确认执行这项高风险写入")
	cancel.Flags().BoolVar(&dryRun, "dry-run", false, "只验证，不改变服务端状态")
	var after int64
	events := &cobra.Command{Use: "events <run-id>", Args: cobra.ExactArgs(1), Short: "显示指定游标之后的不可变任务进度事件", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.RuntimeRunEvent
		if err := client.Dispatch(cmd.Context(), "run.events", map[string]any{"id": args[0], "after": after}, &result); err != nil {
			return err
		}
		return r.writeOK("run.events", result)
	}}
	events.Flags().Int64Var(&after, "after", 0, "仅返回游标大于此值的事件")
	log := &cobra.Command{Use: "log <run-id>", Args: cobra.ExactArgs(1), Short: "显示已保存的任务进度，但不暴露本地智能体输出", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var run domain.RuntimeRun
		if err := client.Dispatch(cmd.Context(), "run.show", map[string]any{"id": args[0]}, &run); err != nil {
			return err
		}
		var events []domain.RuntimeRunEvent
		if err := client.Dispatch(cmd.Context(), "run.events", map[string]any{"id": args[0], "after": 0}, &events); err != nil {
			return err
		}
		return r.writeOK("run.log", map[string]any{"run_id": run.ID, "state": run.State, "progress_label": run.ProgressLabel, "attempt_count": run.AttemptCount, "error_code": run.ErrorCode, "updated_at": run.UpdatedAt, "events": events})
	}}
	cmd.AddCommand(cancel, events, log)
	return cmd
}

func (r *Root) reviewCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "review", Short: "管理绑定客户身份的审核权限"}
	var email string
	var createDryRun bool
	create := &cobra.Command{Use: "create <submission-revision-id>", Args: cobra.ExactArgs(1), Short: "创建有效期为 7 天的客户审批链接", RunE: func(cmd *cobra.Command, args []string) error {
		if createDryRun {
			return r.writeOK("review.create", map[string]any{"dry_run": true, "revision_id": args[0], "reviewer_email": email})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ReviewGrant
		if err := client.Dispatch(cmd.Context(), "review.create", map[string]any{"revision_id": args[0], "reviewer_email": email}, &result); err != nil {
			return err
		}
		return r.writeOK("review.create", result)
	}}
	create.Flags().StringVar(&email, "email", "", "绑定的客户审批人邮箱")
	create.Flags().BoolVar(&createDryRun, "dry-run", false, "只验证，不创建客户审批链接")
	_ = create.MarkFlagRequired("email")
	list := &cobra.Command{Use: "list <submission-revision-id>", Args: cobra.ExactArgs(1), Short: "列出客户审批权限，但不显示密钥摘要", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.ReviewGrant
		if err := client.Dispatch(cmd.Context(), "review.list", map[string]any{"revision_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("review.list", result)
	}}
	var revokeYes, revokeDryRun bool
	revoke := &cobra.Command{Use: "revoke <grant-id>", Args: cobra.ExactArgs(1), Short: "立即撤销一项客户审批权限", RunE: func(cmd *cobra.Command, args []string) error {
		if revokeDryRun {
			return r.writeOK("review.revoke", map[string]any{"dry_run": true, "grant_id": args[0]})
		}
		if !revokeYes {
			return confirmationRequired("撤销后客户审批链接会立即失效")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ReviewGrant
		if err := client.Dispatch(cmd.Context(), "review.revoke", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("review.revoke", result)
	}}
	revoke.Flags().BoolVar(&revokeYes, "yes", false, "确认执行这项高风险写入")
	revoke.Flags().BoolVar(&revokeDryRun, "dry-run", false, "只验证，不撤销审批权限")
	status := &cobra.Command{Use: "status <submission-revision-id>", Args: cobra.ExactArgs(1), Short: "显示提交版本（SubmissionRevision）的客户审核状态", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result app.SubmissionReviewStatus
		if err := client.Dispatch(cmd.Context(), "review.status", map[string]any{"revision_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("review.status", result)
	}}
	cmd.AddCommand(create, list, revoke, status)
	return cmd
}

func (r *Root) resultCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "result", Short: "导入和查看效果观测数据"}
	cmd.AddCommand(r.projectListCommand("list", "result.list", "列出项目观测数据", "project_id"))
	cmd.AddCommand(r.projectListCommand("batches", "result.batches", "列出不可变的效果数据导入批次", "project_id"))
	cmd.AddCommand(r.projectListCommand("ratings", "result.ratings", "列出人工评级决定", "project_id"))
	batchShow := &cobra.Command{Use: "batch-show <batch-id>", Args: cobra.ExactArgs(1), Short: "显示一个导入批次及其观测数据", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.PerformanceImportDetails
		if err := client.Dispatch(cmd.Context(), "result.batch-show", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("result.batch-show", result)
	}}
	cmd.AddCommand(batchShow)

	var dryRun bool
	importCmd := &cobra.Command{Use: "import <json-csv-or-xlsx-file>", Args: cobra.ExactArgs(1), Short: "导入已校验的效果观测数据", RunE: func(cmd *cobra.Command, args []string) error {
		info, err := os.Stat(args[0])
		if err != nil {
			return err
		}
		if info.Size() > 20*1024*1024 {
			return domain.Invalid("RESULT_FILE_TOO_LARGE", "结果文件不能超过 20 MB")
		}
		body, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		inputs, err := parseObservationFile(args[0], body)
		if err != nil {
			return err
		}
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		projectID, err := r.resolveProject(cmd, cfg, client)
		if err != nil {
			return err
		}
		for index := range inputs {
			if inputs[index].ProjectID == "" {
				inputs[index].ProjectID = projectID
			}
		}
		digest := sha256.Sum256(body)
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(args[0])), ".")
		input := app.ImportPerformanceInput{ProjectID: projectID, SourceName: filepath.Base(args[0]), SourceFormat: format, SourceSHA256: hex.EncodeToString(digest[:]), Observations: inputs, DryRun: dryRun}
		var result app.ImportPerformanceResult
		if err := client.Dispatch(cmd.Context(), "result.import", input, &result); err != nil {
			return err
		}
		return r.writeOK("result.import", result)
	}}
	importCmd.Flags().BoolVar(&dryRun, "dry-run", false, "校验整个批次，但不保存数据")
	cmd.AddCommand(importCmd)

	var observationIDs []string
	var rating, reason, nextAction string
	var ratingDryRun bool
	rate := &cobra.Command{Use: "rate <approved_snapshot> <subject-id>", Args: cobra.ExactArgs(2), Short: "创建不可变的人工评级决定", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		projectID, err := r.resolveProject(cmd, cfg, client)
		if err != nil {
			return err
		}
		input := app.CreateRatingDecisionInput{ProjectID: projectID, SubjectType: args[0], SubjectID: args[1], ObservationIDs: observationIDs, Rating: rating, Reason: reason, NextAction: nextAction, DryRun: ratingDryRun}
		var result app.CreateRatingDecisionResult
		if err := client.Dispatch(cmd.Context(), "result.rate", input, &result); err != nil {
			return err
		}
		return r.writeOK("result.rate", result)
	}}
	rate.Flags().StringSliceVar(&observationIDs, "observation", nil, "支持该决定的观测数据 ID，可重复传入")
	rate.Flags().StringVar(&rating, "rating", "", "评级：seed_candidate、repairable、discarded 或 insufficient_sample")
	rate.Flags().StringVar(&reason, "reason", "", "人工评级理由")
	rate.Flags().StringVar(&nextAction, "next-action", "", "明确的下一步操作")
	rate.Flags().BoolVar(&ratingDryRun, "dry-run", false, "只验证，不保存评级决定")
	_ = rate.MarkFlagRequired("observation")
	_ = rate.MarkFlagRequired("rating")
	_ = rate.MarkFlagRequired("reason")
	_ = rate.MarkFlagRequired("next-action")
	cmd.AddCommand(rate)
	return cmd
}

func (r *Root) requestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "request", Short: "只读访问诊断白名单中的资源"}
	get := &cobra.Command{Use: "get <projects|tenants|runs>", Args: cobra.ExactArgs(1), Short: "读取一个白名单集合", RunE: func(cmd *cobra.Command, args []string) error {
		mapping := map[string]string{"projects": "project.list", "tenants": "tenant.list", "runs": "run.list"}
		command, ok := mapping[args[0]]
		if !ok {
			return domain.Invalid("REQUEST_RESOURCE_BLOCKED", "仅允许读取 projects、tenants 或 runs")
		}
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		params := map[string]any{}
		if command == "run.list" {
			id, err := r.resolveProject(cmd, cfg, client)
			if err != nil {
				return err
			}
			params["project_id"] = id
		}
		var result any
		if err := client.Dispatch(cmd.Context(), command, params, &result); err != nil {
			return err
		}
		return r.writeOK("request.get", result)
	}}
	cmd.AddCommand(get)
	return cmd
}

func (r *Root) userClient() (localconfig.Config, *apiclient.Client, string, error) {
	cfg, err := localconfig.Load()
	if err != nil {
		return cfg, nil, "", err
	}
	server := r.resolveServer(cfg)
	token, err := localconfig.UserToken(server)
	if err != nil {
		return cfg, nil, "", &domain.Error{Type: "credential", Subtype: "user", Code: "CLI_LOGIN_REQUIRED", Message: "请先运行 contentcloud auth login --no-wait", ExitCode: 3}
	}
	return cfg, apiclient.New(server, token), token, nil
}

func (r *Root) resolveProject(cmd *cobra.Command, cfg localconfig.Config, client *apiclient.Client) (string, error) {
	if id, err := localconfig.ResolveProject(r.projectID, cfg); err == nil {
		return id, nil
	}
	projects := []domain.Project{}
	if err := client.Dispatch(cmd.Context(), "project.list", map[string]any{}, &projects); err != nil {
		return "", err
	}
	if len(projects) == 1 {
		return projects[0].ID, nil
	}
	return "", domain.Invalid("PROJECT_CONTEXT_REQUIRED", "请用 --project、CONTENTCLOUD_PROJECT_ID 或 .contentcloud/project.json 指定项目")
}

func (r *Root) projectListCommand(use, command, short, projectKey string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		projectID, err := r.resolveProject(cmd, cfg, client)
		if err != nil {
			return err
		}
		var result any
		if err := client.Dispatch(cmd.Context(), command, map[string]any{projectKey: projectID}, &result); err != nil {
			return err
		}
		return r.writeOK(command, result)
	}}
}

func (r *Root) simpleListCommand(use, command, short string, params func(localconfig.Config) map[string]any) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result any
		if err := client.Dispatch(cmd.Context(), command, params(cfg), &result); err != nil {
			return err
		}
		return r.writeOK(command, result)
	}}
}

func (r *Root) idReadCommand(use, command, short, key string) *cobra.Command {
	return &cobra.Command{Use: use, Args: cobra.ExactArgs(1), Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result any
		if err := client.Dispatch(cmd.Context(), command, map[string]any{key: args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK(command, result)
	}}
}

func confirmationRequired(message string) error {
	err := domain.E("confirmation", "high_risk_write", "CONFIRMATION_REQUIRED", message, 10)
	err.Hint = "用户明确确认后，使用原命令追加 --yes"
	return err
}

func mimeForFile(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
