package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	cmd := &cobra.Command{Use: "auth", Short: "Manage user CLI authentication separately from device credentials"}
	var noWait bool
	var deviceCode string
	login := &cobra.Command{Use: "login", Short: "Start or complete browser-confirmed device login", RunE: func(cmd *cobra.Command, args []string) error {
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
	login.Flags().BoolVar(&noWait, "no-wait", false, "start login and return immediately for browser confirmation")
	login.Flags().StringVar(&deviceCode, "device-code", "", "complete a previously approved device login")
	status := &cobra.Command{Use: "status", Short: "Show user authentication without exposing credentials", RunE: func(cmd *cobra.Command, args []string) error {
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
	logout := &cobra.Command{Use: "logout", Short: "Revoke and remove the user CLI credential", RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd := &cobra.Command{Use: "tenant", Short: "Discover tenant context"}
	cmd.AddCommand(r.simpleListCommand("list", "tenant.list", "List accessible tenants", func(_ localconfig.Config) map[string]any { return map[string]any{} }))
	var dryRun bool
	switchCmd := &cobra.Command{Use: "switch <tenant-id>", Args: cobra.ExactArgs(1), Short: "Rotate the CLI credential into another authorized tenant", RunE: func(cmd *cobra.Command, args []string) error {
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
	switchCmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without rotating the CLI credential")
	cmd.AddCommand(switchCmd)
	return cmd
}

func (r *Root) teamCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "team", Short: "Manage fixed-role tenant memberships and invitations"}
	cmd.AddCommand(r.simpleListCommand("members", "membership.list", "List tenant members", func(_ localconfig.Config) map[string]any { return map[string]any{} }))
	invites := &cobra.Command{Use: "invites", Short: "List tenant invitations", RunE: func(cmd *cobra.Command, args []string) error {
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
	invite := &cobra.Command{Use: "invite <email>", Args: cobra.ExactArgs(1), Short: "Create a 72-hour tenant invitation", RunE: func(cmd *cobra.Command, args []string) error {
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
	invite.Flags().StringVar(&inviteRole, "role", "viewer", "tenant_admin, project_manager, strategist, editor, reviewer, or viewer")
	invite.Flags().BoolVar(&inviteDryRun, "dry-run", false, "validate without creating an invitation")
	var acceptDryRun bool
	accept := &cobra.Command{Use: "accept <invite-token>", Args: cobra.ExactArgs(1), Short: "Accept an invitation bound to the logged-in email", RunE: func(cmd *cobra.Command, args []string) error {
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
	accept.Flags().BoolVar(&acceptDryRun, "dry-run", false, "validate without accepting the invitation")
	var revokeInviteYes, revokeInviteDryRun bool
	revokeInvite := &cobra.Command{Use: "revoke-invite <invite-id>", Args: cobra.ExactArgs(1), Short: "Revoke one pending tenant invitation", RunE: func(cmd *cobra.Command, args []string) error {
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
	revokeInvite.Flags().BoolVar(&revokeInviteYes, "yes", false, "confirm this high-risk write")
	revokeInvite.Flags().BoolVar(&revokeInviteDryRun, "dry-run", false, "validate without revoking the invitation")
	var roleDryRun bool
	setRole := &cobra.Command{Use: "set-role <user-id> <role>", Args: cobra.ExactArgs(2), Short: "Assign one fixed tenant role", RunE: func(cmd *cobra.Command, args []string) error {
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
	setRole.Flags().BoolVar(&roleDryRun, "dry-run", false, "validate without changing the role")
	var revokeYes, revokeDryRun bool
	revoke := &cobra.Command{Use: "revoke <user-id>", Args: cobra.ExactArgs(1), Short: "Revoke membership and active tenant sessions", RunE: func(cmd *cobra.Command, args []string) error {
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
	revoke.Flags().BoolVar(&revokeYes, "yes", false, "confirm this high-risk write")
	revoke.Flags().BoolVar(&revokeDryRun, "dry-run", false, "validate without revoking membership")
	cmd.AddCommand(invites, invite, accept, revokeInvite, setRole, revoke)
	return cmd
}

func (r *Root) fullProjectCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Discover, resolve, and inspect projects"}
	cmd.AddCommand(r.simpleListCommand("list", "project.list", "List accessible projects", func(_ localconfig.Config) map[string]any { return map[string]any{} }))
	show := &cobra.Command{Use: "show [project-id]", Args: cobra.MaximumNArgs(1), Short: "Show one project", RunE: func(cmd *cobra.Command, args []string) error {
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
	resolve := &cobra.Command{Use: "resolve <name-or-slug>", Args: cobra.ExactArgs(1), Short: "Resolve a human project name or slug to a stable ID", RunE: func(cmd *cobra.Command, args []string) error {
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
	create := &cobra.Command{Use: "create", Short: "Create one single-product project", RunE: func(cmd *cobra.Command, args []string) error {
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
	create.Flags().StringVar(&brand, "brand", "", "brand name")
	create.Flags().StringVar(&product, "product", "", "single focus product name")
	create.Flags().StringVar(&channel, "channel", "", "target channel")
	create.Flags().StringVar(&objective, "objective", "", "single stage objective")
	create.Flags().StringVar(&owner, "owner", "", "project owner display name")
	create.Flags().StringVar(&reviewer, "reviewer", "", "internal reviewer display name")
	create.Flags().StringVar(&approver, "client-approver", "", "client approver display name")
	create.Flags().StringVar(&templateID, "template", "", "tenant project template ID")
	create.Flags().BoolVar(&createDryRun, "dry-run", false, "validate without creating a project")
	_ = create.MarkFlagRequired("brand")
	_ = create.MarkFlagRequired("product")
	var lifecycleYes, lifecycleDryRun bool
	lifecycle := func(action string) *cobra.Command {
		var rowVersion int
		command := &cobra.Command{Use: action + " <project-id>", Args: cobra.ExactArgs(1), Short: action + " a project with optimistic locking", RunE: func(cmd *cobra.Command, args []string) error {
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
		command.Flags().IntVar(&rowVersion, "row-version", 0, "required current project row version")
		command.Flags().BoolVar(&lifecycleYes, "yes", false, "confirm this high-risk write")
		command.Flags().BoolVar(&lifecycleDryRun, "dry-run", false, "validate without changing project state")
		_ = command.MarkFlagRequired("row-version")
		return command
	}
	templates := &cobra.Command{Use: "templates", Short: "List sanitized tenant project templates", RunE: func(cmd *cobra.Command, args []string) error {
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
	createTemplate := &cobra.Command{Use: "create", Short: "Create a sanitized project template without customer facts or assets", RunE: func(cmd *cobra.Command, args []string) error {
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
	createTemplate.Flags().StringVar(&templateName, "name", "", "template name")
	createTemplate.Flags().StringVar(&templateChannel, "channel", "douyin", "default target channel")
	createTemplate.Flags().StringVar(&templateObjective, "objective", "", "default stage objective")
	createTemplate.Flags().BoolVar(&templateDryRun, "dry-run", false, "validate without creating a template")
	_ = createTemplate.MarkFlagRequired("name")
	templates.AddCommand(createTemplate)
	var updateRowVersion int
	var updateBrand, updateProduct, updateChannel, updateObjective, updateOwner, updateReviewer, updateApprover string
	var updateDryRun bool
	update := &cobra.Command{Use: "update <project-id>", Args: cobra.ExactArgs(1), Short: "Update project metadata with optimistic locking", RunE: func(cmd *cobra.Command, args []string) error {
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
	update.Flags().IntVar(&updateRowVersion, "row-version", 0, "required current project row version")
	update.Flags().StringVar(&updateBrand, "brand", "", "brand name")
	update.Flags().StringVar(&updateProduct, "product", "", "single focus product name")
	update.Flags().StringVar(&updateChannel, "channel", "", "target channel")
	update.Flags().StringVar(&updateObjective, "objective", "", "stage objective")
	update.Flags().StringVar(&updateOwner, "owner", "", "project owner display name")
	update.Flags().StringVar(&updateReviewer, "reviewer", "", "internal reviewer display name")
	update.Flags().StringVar(&updateApprover, "client-approver", "", "client approver display name")
	update.Flags().BoolVar(&updateDryRun, "dry-run", false, "validate without changing the project")
	_ = update.MarkFlagRequired("row-version")
	cmd.AddCommand(show, resolve, create, update, lifecycle("archive"), lifecycle("restore"), templates)
	return cmd
}

func (r *Root) deviceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "device", Short: "Inspect and revoke connected creative runtimes"}
	cmd.AddCommand(r.projectListCommand("list", "device.list", "List devices authorized for a project", "project_id"))
	var connectDryRun bool
	connectCreate := &cobra.Command{Use: "connect-create [project-id]", Args: cobra.MaximumNArgs(1), Short: "Create a 10-minute project connection session", RunE: func(cmd *cobra.Command, args []string) error {
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
	connectCreate.Flags().BoolVar(&connectDryRun, "dry-run", false, "validate without creating a connection session")
	connectShow := &cobra.Command{Use: "connect-show <session-id>", Args: cobra.ExactArgs(1), Short: "Show a project connection session", RunE: func(cmd *cobra.Command, args []string) error {
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
	connectCancel := &cobra.Command{Use: "connect-cancel <session-id>", Args: cobra.ExactArgs(1), Short: "Cancel a waiting project connection session", RunE: func(cmd *cobra.Command, args []string) error {
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
	connectCancel.Flags().BoolVar(&cancelConnectYes, "yes", false, "confirm this high-risk write")
	connectCancel.Flags().BoolVar(&cancelConnectDryRun, "dry-run", false, "validate without canceling the connection session")
	show := &cobra.Command{Use: "show <device-id>", Args: cobra.ExactArgs(1), Short: "Show one connected creative runtime", RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd.AddCommand(show, connectCreate, connectShow, connectCancel, r.deviceGrantCommand("attach", "Authorize a device for the selected project"), r.deviceGrantCommand("detach", "Remove a device from the selected project"))
	var yes, dryRun bool
	revoke := &cobra.Command{Use: "revoke <device-id>", Args: cobra.ExactArgs(1), Short: "Revoke a device immediately", RunE: func(cmd *cobra.Command, args []string) error {
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
	revoke.Flags().BoolVar(&yes, "yes", false, "confirm this high-risk write")
	revoke.Flags().BoolVar(&dryRun, "dry-run", false, "validate without changing server state")
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
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without changing server state")
	if action == "detach" {
		cmd.Flags().BoolVar(&yes, "yes", false, "confirm this high-risk write")
	}
	return cmd
}

func (r *Root) sourceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "source", Short: "Upload and inspect governed source files"}
	cmd.AddCommand(r.projectListCommand("list", "source.list", "List project sources", "project_id"))
	var name, sourceType, mimeType string
	var uploadDryRun bool
	upload := &cobra.Command{Use: "upload <file>", Args: cobra.ExactArgs(1), Short: "Upload one supported immutable source revision", RunE: func(cmd *cobra.Command, args []string) error {
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
	upload.Flags().StringVar(&name, "name", "", "logical source name")
	upload.Flags().StringVar(&sourceType, "type", "brand_manual", "source type")
	upload.Flags().StringVar(&mimeType, "mime", "", "declared MIME; inferred from extension by default")
	upload.Flags().BoolVar(&uploadDryRun, "dry-run", false, "validate without uploading or changing server state")
	status := &cobra.Command{Use: "status <revision-id>", Args: cobra.ExactArgs(1), Short: "Show immutable source revision processing status", RunE: func(cmd *cobra.Command, args []string) error {
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
	revisions := r.idReadCommand("revisions <source-id>", "source.revisions", "List immutable revisions for one logical source", "source_id")
	impact := r.idReadCommand("impact <source-id>", "source.impact", "Show knowledge, Brief, and script versions affected by a source", "source_id")
	var reviseMIME string
	var reviseDryRun bool
	revise := &cobra.Command{Use: "revise <source-id> <file>", Args: cobra.ExactArgs(2), Short: "Upload a new immutable revision for an existing logical source", RunE: func(cmd *cobra.Command, args []string) error {
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
	revise.Flags().StringVar(&reviseMIME, "mime", "", "declared MIME; inferred from extension by default")
	revise.Flags().BoolVar(&reviseDryRun, "dry-run", false, "validate without uploading or changing server state")
	var evidenceDryRun bool
	evidenceReview := &cobra.Command{Use: "evidence-review <evidence-id> <accept|reject>", Args: cobra.ExactArgs(2), Short: "Record a human decision for one extracted evidence span", RunE: func(cmd *cobra.Command, args []string) error {
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
	evidenceReview.Flags().BoolVar(&evidenceDryRun, "dry-run", false, "validate without changing server state")
	cmd.AddCommand(upload, status, revisions, impact, revise, evidenceReview)
	return cmd
}

func (r *Root) assetCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "asset", Short: "Govern generation assets and their usage rights"}
	cmd.AddCommand(r.projectListCommand("list", "asset.list", "List governed assets for a project", "project_id"))

	var name, assetType, sourceRevisionID, usageMode string
	var createDryRun bool
	create := &cobra.Command{Use: "create", Short: "Register a source revision as a governed asset", RunE: func(command *cobra.Command, args []string) error {
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
	create.Flags().StringVar(&name, "name", "", "asset display name")
	create.Flags().StringVar(&assetType, "type", "product_image", "product_image, brand_mark, packaging, person, location, audio, or other")
	create.Flags().StringVar(&sourceRevisionID, "source-revision", "", "ready source revision containing the immutable asset bytes")
	create.Flags().StringVar(&usageMode, "usage", "analysis_only", "analysis_only, generation_reference, or owned")
	create.Flags().BoolVar(&createDryRun, "dry-run", false, "validate locally without changing server state")
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("source-revision")

	rights := &cobra.Command{Use: "rights <asset-id>", Args: cobra.ExactArgs(1), Short: "List rights records for an asset", RunE: func(command *cobra.Command, args []string) error {
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
	rightsCreate := &cobra.Command{Use: "rights-create <asset-id>", Args: cobra.ExactArgs(1), Short: "Create a reviewable rights record", RunE: func(command *cobra.Command, args []string) error {
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
	rightsCreate.Flags().StringVar(&holder, "holder", "", "rights holder")
	rightsCreate.Flags().StringVar(&rightsType, "type", "owned", "owned, licensed_generation, licensed_edit, or public_domain")
	rightsCreate.Flags().StringSliceVar(&territories, "territory", []string{"CN"}, "allowed territory; repeat or comma-separate")
	rightsCreate.Flags().StringSliceVar(&channels, "channel", nil, "allowed channel; repeat or comma-separate")
	rightsCreate.Flags().StringVar(&proofRevision, "proof-source-revision", "", "ready source revision containing rights proof")
	rightsCreate.Flags().StringVar(&validFrom, "valid-from", "", "optional RFC3339 start time")
	rightsCreate.Flags().StringVar(&validUntil, "valid-until", "", "optional RFC3339 end time")
	rightsCreate.Flags().StringSliceVar(&restrictions, "restriction", nil, "usage restriction; repeat or comma-separate")
	rightsCreate.Flags().BoolVar(&rightsDryRun, "dry-run", false, "validate locally without changing server state")
	_ = rightsCreate.MarkFlagRequired("holder")
	_ = rightsCreate.MarkFlagRequired("channel")
	_ = rightsCreate.MarkFlagRequired("proof-source-revision")

	var reviewDryRun bool
	review := &cobra.Command{Use: "rights-review <rights-id> <approve|reject>", Args: cobra.ExactArgs(2), Short: "Record a reviewer decision for one rights record", RunE: func(command *cobra.Command, args []string) error {
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
	review.Flags().BoolVar(&reviewDryRun, "dry-run", false, "validate without changing server state")
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
	cmd := &cobra.Command{Use: "knowledge", Short: "Inspect and review governed knowledge"}
	cmd.AddCommand(r.projectListCommand("list", "knowledge.list", "List project knowledge", "project_id"))
	cmd.AddCommand(r.projectListCommand("conflicts", "knowledge.conflicts", "List explicit knowledge conflicts", "project_id"))
	cmd.AddCommand(r.projectListCommand("decisions", "knowledge.decisions", "List brand fact decision requests", "project_id"))
	var sourceRevisionIDs []string
	var outputCount int
	var idempotencyKey string
	var extractDryRun bool
	extract := &cobra.Command{Use: "extract", Short: "Queue a local evidence-grounded knowledge extraction run", RunE: func(command *cobra.Command, args []string) error {
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
		var result domain.TaskRun
		if err := client.Dispatch(command.Context(), "knowledge.extract", input, &result); err != nil {
			return err
		}
		return r.writeOK("knowledge.extract", result)
	}}
	extract.Flags().StringSliceVar(&sourceRevisionIDs, "source-revision", nil, "ready source revision ID; repeat or comma-separate")
	extract.Flags().IntVar(&outputCount, "count", 20, "maximum number of knowledge candidates, 1-20")
	extract.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "stable key for safe retries")
	extract.Flags().BoolVar(&extractDryRun, "dry-run", false, "validate without queueing a server task")
	_ = extract.MarkFlagRequired("source-revision")
	var dryRun bool
	review := &cobra.Command{Use: "review <id> <approve|reject|conflict|return>", Args: cobra.ExactArgs(2), Short: "Record one human knowledge decision", RunE: func(cmd *cobra.Command, args []string) error {
		if dryRun {
			return r.writeOK("knowledge.review", map[string]any{"dry_run": true, "id": args[0], "decision": args[1]})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.KnowledgeItem
		if err := client.Dispatch(cmd.Context(), "knowledge.review", map[string]any{"id": args[0], "decision": args[1]}, &result); err != nil {
			return err
		}
		return r.writeOK("knowledge.review", result)
	}}
	review.Flags().BoolVar(&dryRun, "dry-run", false, "validate without changing server state")
	var selectedKnowledgeID, notes string
	var resolveDryRun bool
	resolve := &cobra.Command{Use: "resolve <decision-request-id>", Args: cobra.ExactArgs(1), Short: "Resolve a conflict by selecting one preserved knowledge value", RunE: func(command *cobra.Command, args []string) error {
		params := map[string]any{"id": args[0], "selected_knowledge_id": selectedKnowledgeID, "notes": notes}
		if resolveDryRun {
			params["dry_run"] = true
			return r.writeOK("knowledge.decision.resolve", params)
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.DecisionRequest
		if err := client.Dispatch(command.Context(), "knowledge.decision.resolve", params, &result); err != nil {
			return err
		}
		return r.writeOK("knowledge.decision.resolve", result)
	}}
	resolve.Flags().StringVar(&selectedKnowledgeID, "select", "", "knowledge item ID selected by the human decision")
	resolve.Flags().StringVar(&notes, "notes", "", "decision rationale")
	resolve.Flags().BoolVar(&resolveDryRun, "dry-run", false, "validate without changing server state")
	_ = resolve.MarkFlagRequired("select")
	cmd.AddCommand(r.idReadCommand("show <knowledge-id>", "knowledge.show", "Show one knowledge item with evidence", "id"), extract, review, resolve)
	return cmd
}

func (r *Root) briefCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "brief", Short: "Inspect and review immutable Brief versions"}
	cmd.AddCommand(r.projectListCommand("list", "brief.list", "List project Brief versions", "project_id"))
	show := r.idReadCommand("show <brief-id>", "brief.show", "Show one Brief version", "id")

	var createFile string
	var createDryRun bool
	create := &cobra.Command{Use: "create", Short: "Create an immutable Brief version from a JSON document", RunE: func(command *cobra.Command, args []string) error {
		input, err := readBriefInput(createFile)
		if err != nil {
			return err
		}
		if createDryRun {
			if err := r.resolveBriefInputProject(command, input, nil, localconfig.Config{}); err != nil {
				return err
			}
			return r.writeOK("brief.create", map[string]any{"dry_run": true, "input": input})
		}
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		if err := r.resolveBriefInputProject(command, input, client, cfg); err != nil {
			return err
		}
		var result domain.BriefVersion
		if err := client.Dispatch(command.Context(), "brief.create", input, &result); err != nil {
			return err
		}
		return r.writeOK("brief.create", result)
	}}
	create.Flags().StringVar(&createFile, "file", "", "path to a Brief input JSON document")
	create.Flags().BoolVar(&createDryRun, "dry-run", false, "validate the local document without changing server state")
	_ = create.MarkFlagRequired("file")

	var reviseFile, revisionReason string
	var reviseDryRun bool
	revise := &cobra.Command{Use: "revise <brief-id>", Args: cobra.ExactArgs(1), Short: "Create a new immutable version that supersedes a Brief", RunE: func(command *cobra.Command, args []string) error {
		input, err := readBriefInput(reviseFile)
		if err != nil {
			return err
		}
		input.SupersedesID = args[0]
		input.RevisionReason = strings.TrimSpace(revisionReason)
		if reviseDryRun {
			if err := r.resolveBriefInputProject(command, input, nil, localconfig.Config{}); err != nil {
				return err
			}
			return r.writeOK("brief.revise", map[string]any{"dry_run": true, "input": input})
		}
		cfg, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		if err := r.resolveBriefInputProject(command, input, client, cfg); err != nil {
			return err
		}
		var result domain.BriefVersion
		if err := client.Dispatch(command.Context(), "brief.create", input, &result); err != nil {
			return err
		}
		return r.writeOK("brief.revise", result)
	}}
	revise.Flags().StringVar(&reviseFile, "file", "", "path to the complete replacement Brief input JSON document")
	revise.Flags().StringVar(&revisionReason, "reason", "", "required reason for creating a replacement version")
	revise.Flags().BoolVar(&reviseDryRun, "dry-run", false, "validate the local document without changing server state")
	_ = revise.MarkFlagRequired("file")
	_ = revise.MarkFlagRequired("reason")

	reviewCommand := func(use, outputCommand, decision, short string, reasonRequired bool) *cobra.Command {
		var reason string
		var dryRun bool
		command := &cobra.Command{Use: use + " <brief-id>", Args: cobra.ExactArgs(1), Short: short, RunE: func(command *cobra.Command, args []string) error {
			if reasonRequired && strings.TrimSpace(reason) == "" {
				return domain.Invalid("BRIEF_RETURN_REASON_REQUIRED", "退回 Brief 必须填写原因")
			}
			params := map[string]any{"id": args[0], "decision": decision, "reason": strings.TrimSpace(reason)}
			if dryRun {
				params["dry_run"] = true
				return r.writeOK(outputCommand, params)
			}
			_, client, _, err := r.userClient()
			if err != nil {
				return err
			}
			var result domain.BriefVersion
			if err := client.Dispatch(command.Context(), "brief.review", params, &result); err != nil {
				return err
			}
			return r.writeOK(outputCommand, result)
		}}
		if reasonRequired {
			command.Flags().StringVar(&reason, "reason", "", "required review return reason")
			_ = command.MarkFlagRequired("reason")
		}
		command.Flags().BoolVar(&dryRun, "dry-run", false, "validate without changing server state")
		return command
	}

	var dryRun bool
	approve := &cobra.Command{Use: "approve <brief-id>", Args: cobra.ExactArgs(1), Short: "Approve an internally reviewed Brief", RunE: func(cmd *cobra.Command, args []string) error {
		if dryRun {
			return r.writeOK("brief.approve", map[string]any{"dry_run": true, "brief_id": args[0]})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.BriefVersion
		if err := client.Dispatch(cmd.Context(), "brief.review", map[string]any{"id": args[0], "decision": "approve"}, &result); err != nil {
			return err
		}
		return r.writeOK("brief.approve", result)
	}}
	approve.Flags().BoolVar(&dryRun, "dry-run", false, "validate without changing server state")
	cmd.AddCommand(show, create, revise, reviewCommand("submit", "brief.submit", "submit", "Submit a draft Brief for internal review", false), reviewCommand("return", "brief.return", "return", "Return a Brief with a required reason", true), approve)
	return cmd
}

func readBriefInput(path string) (*app.CreateBriefInput, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var input app.CreateBriefInput
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, domain.Invalid("BRIEF_INPUT_INVALID", "Brief JSON 无效或包含未知字段: "+err.Error())
	}
	return &input, nil
}

func (r *Root) resolveBriefInputProject(command *cobra.Command, input *app.CreateBriefInput, client *apiclient.Client, cfg localconfig.Config) error {
	if r.projectID == "" && input.ProjectID != "" {
		return nil
	}
	if client == nil {
		loaded, err := localconfig.Load()
		if err != nil {
			return err
		}
		projectID, err := localconfig.ResolveProject(r.projectID, loaded)
		if err != nil {
			return domain.Invalid("PROJECT_CONTEXT_REQUIRED", "Brief JSON 缺少 project_id，且未解析到 --project 或本地项目上下文")
		}
		input.ProjectID = projectID
		return nil
	}
	projectID, err := r.resolveProject(command, cfg, client)
	if err != nil {
		return err
	}
	input.ProjectID = projectID
	return nil
}

func (r *Root) runCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "run", Short: "Create and inspect local creative task runs"}
	cmd.AddCommand(r.projectListCommand("list", "run.list", "List project runs", "project_id"), r.idReadCommand("show <run-id>", "run.show", "Show one run", "id"), r.idReadCommand("attempts <run-id>", "run.attempts", "Show immutable execution attempts", "id"))
	var idem string
	create := &cobra.Command{Use: "create <brief-id>", Args: cobra.ExactArgs(1), Short: "Create one script generation run", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.TaskRun
		if err := client.Dispatch(cmd.Context(), "run.create", map[string]any{"brief_id": args[0], "idempotency_key": idem}, &result); err != nil {
			return err
		}
		return r.writeOK("run.create", result)
	}}
	create.Flags().StringVar(&idem, "idempotency-key", "", "stable key for safe retries")
	var yes, dryRun bool
	cancel := &cobra.Command{Use: "cancel <run-id>", Args: cobra.ExactArgs(1), Short: "Request cancellation of a queued or running task", RunE: func(cmd *cobra.Command, args []string) error {
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
		var result domain.TaskRun
		if err := client.Dispatch(cmd.Context(), "run.cancel", map[string]any{"id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("run.cancel", result)
	}}
	cancel.Flags().BoolVar(&yes, "yes", false, "confirm this high-risk write")
	cancel.Flags().BoolVar(&dryRun, "dry-run", false, "validate without changing server state")
	log := &cobra.Command{Use: "log <run-id>", Args: cobra.ExactArgs(1), Short: "Show persisted run progress without exposing local Agent output", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var run domain.TaskRun
		if err := client.Dispatch(cmd.Context(), "run.show", map[string]any{"id": args[0]}, &run); err != nil {
			return err
		}
		return r.writeOK("run.log", map[string]any{"run_id": run.ID, "state": run.State, "progress_label": run.ProgressLabel, "attempt_count": run.AttemptCount, "error_code": run.ErrorCode, "updated_at": run.UpdatedAt})
	}}
	cmd.AddCommand(create, cancel, log)
	return cmd
}

func (r *Root) scriptCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "script", Short: "Inspect and review canonical Script Packages"}
	cmd.AddCommand(r.projectListCommand("list", "script.list", "List project scripts", "project_id"), r.idReadCommand("show <script-id>", "script.show", "Show one Script Package", "id"))
	changeCommand := func(use, changeType, short string) *cobra.Command {
		var briefID, hypothesis, reason, idempotencyKey string
		var invariants, changedFields []string
		var dryRun bool
		command := &cobra.Command{Use: use + " <baseline-version-id>", Args: cobra.ExactArgs(1), Short: short, RunE: func(command *cobra.Command, args []string) error {
			input := app.CreateScriptChangeRunInput{BriefVersionID: briefID, ChangeType: changeType, InvariantFields: invariants, ChangedFields: changedFields, Hypothesis: hypothesis, RevisionReason: reason, IdempotencyKey: idempotencyKey}
			if input.IdempotencyKey == "" {
				input.IdempotencyKey = domain.NewID()
			}
			if dryRun {
				return r.writeOK("script.change.create", map[string]any{"dry_run": true, "baseline_script_version_id": args[0], "input": input})
			}
			_, client, _, err := r.userClient()
			if err != nil {
				return err
			}
			params := struct {
				BaselineVersionID string `json:"baseline_script_version_id"`
				app.CreateScriptChangeRunInput
			}{BaselineVersionID: args[0], CreateScriptChangeRunInput: input}
			var result domain.TaskRun
			if err := client.Dispatch(command.Context(), "script.change.create", params, &result); err != nil {
				return err
			}
			return r.writeOK("script.change.create", result)
		}}
		command.Flags().StringVar(&briefID, "brief", "", "approved BriefVersion ID; defaults to the current approved version")
		command.Flags().StringSliceVar(&invariants, "invariant", nil, "JSON Pointer that must remain unchanged; repeat or comma-separate")
		command.Flags().StringSliceVar(&changedFields, "changed-field", nil, "JSON Pointer expected to change; repeat or comma-separate")
		command.Flags().StringVar(&hypothesis, "hypothesis", "", "experiment hypothesis; required for a variant")
		command.Flags().StringVar(&reason, "reason", "", "required reason for creating the new immutable version")
		command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "stable key for safe retries")
		command.Flags().BoolVar(&dryRun, "dry-run", false, "validate local arguments without creating a run")
		_ = command.MarkFlagRequired("reason")
		if changeType == "variant" {
			_ = command.MarkFlagRequired("changed-field")
			_ = command.MarkFlagRequired("hypothesis")
		}
		return command
	}
	cmd.AddCommand(changeCommand("revise", "revision", "Create a local Agent run for a new immutable script revision"), changeCommand("variant", "variant", "Create a single-variable local Agent variant run"))
	var conclusion, assignee string
	var dryRun bool
	review := &cobra.Command{Use: "review <script-id> <submit|approve_internal|return>", Args: cobra.ExactArgs(2), Short: "Record one internal script review transition", RunE: func(cmd *cobra.Command, args []string) error {
		if (args[1] == "approve_internal" || args[1] == "return") && strings.TrimSpace(conclusion) == "" {
			return domain.Invalid("REVIEW_CONCLUSION_REQUIRED", "内审批准或退回必须填写 --conclusion")
		}
		if args[1] == "return" && strings.TrimSpace(assignee) == "" {
			return domain.Invalid("REVIEW_ASSIGNEE_REQUIRED", "内审退回必须填写 --assignee")
		}
		if dryRun {
			return r.writeOK("script.review", map[string]any{"dry_run": true, "id": args[0], "decision": args[1], "conclusion": conclusion, "assignee_user_id": assignee})
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result domain.ScriptVersion
		if err := client.Dispatch(cmd.Context(), "script.review", map[string]any{"id": args[0], "decision": args[1], "conclusion": conclusion, "assignee_user_id": assignee}, &result); err != nil {
			return err
		}
		return r.writeOK("script.review", result)
	}}
	review.Flags().StringVar(&conclusion, "conclusion", "", "whole-version review conclusion")
	review.Flags().StringVar(&assignee, "assignee", "", "tenant member user ID responsible for a returned revision")
	review.Flags().BoolVar(&dryRun, "dry-run", false, "validate without changing server state")
	cycles := &cobra.Command{Use: "cycles <script-id>", Args: cobra.ExactArgs(1), Short: "List immutable review cycles for a ScriptVersion", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []domain.ReviewCycle
		if err := client.Dispatch(cmd.Context(), "review_cycle.list", map[string]any{"script_id": args[0]}, &result); err != nil {
			return err
		}
		return r.writeOK("review_cycle.list", result)
	}}
	cmd.AddCommand(review, cycles)
	return cmd
}

func (r *Root) reviewCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "review", Short: "Create customer-bound review grants"}
	var email string
	var createDryRun bool
	create := &cobra.Command{Use: "create <submission-revision-id>", Args: cobra.ExactArgs(1), Short: "Create a seven-day customer approval link", RunE: func(cmd *cobra.Command, args []string) error {
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
	create.Flags().StringVar(&email, "email", "", "bound customer approver email")
	create.Flags().BoolVar(&createDryRun, "dry-run", false, "validate without creating a customer approval link")
	_ = create.MarkFlagRequired("email")
	list := &cobra.Command{Use: "list <submission-revision-id>", Args: cobra.ExactArgs(1), Short: "List customer approval grants without secret hashes", RunE: func(cmd *cobra.Command, args []string) error {
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
	revoke := &cobra.Command{Use: "revoke <grant-id>", Args: cobra.ExactArgs(1), Short: "Revoke a customer approval grant immediately", RunE: func(cmd *cobra.Command, args []string) error {
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
	revoke.Flags().BoolVar(&revokeYes, "yes", false, "confirm this high-risk write")
	revoke.Flags().BoolVar(&revokeDryRun, "dry-run", false, "validate without revoking the grant")
	status := &cobra.Command{Use: "status <submission-revision-id>", Args: cobra.ExactArgs(1), Short: "Show the customer review state for a SubmissionRevision", RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd := &cobra.Command{Use: "result", Short: "Import and inspect performance observations"}
	cmd.AddCommand(r.projectListCommand("list", "result.list", "List project observations", "project_id"))
	cmd.AddCommand(r.projectListCommand("batches", "result.batches", "List immutable performance import batches", "project_id"))
	cmd.AddCommand(r.projectListCommand("ratings", "result.ratings", "List manual rating decisions", "project_id"))
	batchShow := &cobra.Command{Use: "batch-show <batch-id>", Args: cobra.ExactArgs(1), Short: "Show one import batch and its observations", RunE: func(cmd *cobra.Command, args []string) error {
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
	importCmd := &cobra.Command{Use: "import <json-csv-or-xlsx-file>", Args: cobra.ExactArgs(1), Short: "Import validated performance observations", RunE: func(cmd *cobra.Command, args []string) error {
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
	importCmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate the entire batch without persisting it")
	cmd.AddCommand(importCmd)

	var observationIDs []string
	var rating, reason, nextAction string
	var ratingDryRun bool
	rate := &cobra.Command{Use: "rate <approved_snapshot|content_framework|shot_pattern> <subject-id>", Args: cobra.ExactArgs(2), Short: "Create an immutable human rating decision", RunE: func(cmd *cobra.Command, args []string) error {
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
	rate.Flags().StringSliceVar(&observationIDs, "observation", nil, "observation ID supporting the decision (repeatable)")
	rate.Flags().StringVar(&rating, "rating", "", "seed_candidate, repairable, discarded, or insufficient_sample")
	rate.Flags().StringVar(&reason, "reason", "", "human rationale for this rating")
	rate.Flags().StringVar(&nextAction, "next-action", "", "explicit next action")
	rate.Flags().BoolVar(&ratingDryRun, "dry-run", false, "validate without persisting the decision")
	_ = rate.MarkFlagRequired("observation")
	_ = rate.MarkFlagRequired("rating")
	_ = rate.MarkFlagRequired("reason")
	_ = rate.MarkFlagRequired("next-action")
	cmd.AddCommand(rate)
	return cmd
}

func (r *Root) requestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "request", Short: "Read-only allowlisted diagnostic escape hatch"}
	get := &cobra.Command{Use: "get <projects|tenants|runs>", Args: cobra.ExactArgs(1), Short: "Read one allowlisted collection", RunE: func(cmd *cobra.Command, args []string) error {
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
