package httpapi

import (
	"bytes"
	"fmt"
	"html/template"
	"mime"
	"net/http"
	"strconv"
	"strings"
	texttemplate "text/template"

	"github.com/limecloud/contentcloud/internal/codexplugin"
)

const (
	codexGuideVersion       = "0.10.0"
	codexGuideSchemaVersion = "contentcloud.codex-guide/1.0"
	codexGuideVary          = "Accept, Sec-Fetch-Mode, Sec-Fetch-Dest"
)

type codexGuideStep struct {
	Number   int
	Title    string
	Summary  string
	Commands []string
	Notes    []string
}

type codexGuide struct {
	SchemaVersion      string
	Version            string
	MarketplaceName    string
	MarketplaceSource  string
	MarketplaceRef     string
	PluginID           string
	PluginVersion      string
	BootstrapPath      string
	LoginPath          string
	ContextTool        string
	DoctorTool         string
	OpenViewTool       string
	RecoveryPrompt     string
	Steps              []codexGuideStep
	SecurityBoundaries []string
}

func newCodexGuide() codexGuide {
	spec := codexplugin.DefaultSpec(codexGuideVersion)
	cli := fmt.Sprintf("npx --yes @limecloud/contentcloud@%s", codexGuideVersion)
	bootstrapPath := "/api/bootstrap"
	contextTool := "workspace_context"
	doctorTool := "workspace_doctor"
	openViewTool := "contentcloud_open_project_view"
	return codexGuide{
		SchemaVersion:     codexGuideSchemaVersion,
		Version:           codexGuideVersion,
		MarketplaceName:   spec.MarketplaceName,
		MarketplaceSource: spec.MarketplaceSource,
		MarketplaceRef:    spec.MarketplaceRef,
		PluginID:          spec.PluginID,
		PluginVersion:     spec.PluginVersion,
		BootstrapPath:     bootstrapPath,
		LoginPath:         "/login",
		ContextTool:       contextTool,
		DoctorTool:        doctorTool,
		OpenViewTool:      openViewTool,
		RecoveryPrompt:    codexplugin.RecoveryPrompt(spec),
		Steps: []codexGuideStep{
			{
				Number:  1,
				Title:   "确认运行宿主",
				Summary: "先确认当前会话运行在用户电脑上的 Codex Desktop 或具备本地配置权限的 Codex CLI。",
				Notes: []string{
					"远程 Web、只读沙箱或隔离宿主不得修改用户电脑上的 Plugin 配置。",
					"读取本指南不代表用户授权安装、登录、写文件或连接项目。",
				},
			},
			{
				Number:  2,
				Title:   "安装固定 Plugin",
				Summary: "仅在用户确认后安装固定 Marketplace ref 与固定 Plugin ID；项目 bootstrap 也会验证同一组身份。",
				Commands: []string{
					fmt.Sprintf("codex plugin marketplace add %s --ref %s --json", spec.MarketplaceSource, spec.MarketplaceRef),
					fmt.Sprintf("codex plugin add %s --json", spec.PluginID),
				},
				Notes: []string{
					"不要替换 Marketplace 来源、ref、Plugin ID 或版本。",
					"命令成功只表示安装完成；当前对话不会热加载新的 Skill 或 MCP Tool。",
				},
			},
			{
				Number:  3,
				Title:   "登录 ContentCloud",
				Summary: "在此站点登录。需要 CLI 用户会话时，从本机发起浏览器确认流程。",
				Commands: []string{
					fmt.Sprintf("%s --server-url <CONTENTCLOUD_ORIGIN> --json auth login --no-wait", cli),
				},
				Notes: []string{
					"只打开命令返回的同源验证页面，并由用户在浏览器中确认。",
					"不要把登录结果、浏览器会话或本机安全存储内容写入聊天、URL 或 Workspace 文件。",
				},
			},
			{
				Number:  4,
				Title:   "连接项目 Workspace",
				Summary: "在 ContentCloud 中选择或创建项目，从项目的“接入与初始化”页面取得公开 ConnectSession ID，再执行同一个可核对的 bootstrap 事务。",
				Commands: []string{
					fmt.Sprintf("curl -fsS -H 'Accept: text/markdown' <CONTENTCLOUD_ORIGIN>%s", bootstrapPath),
					fmt.Sprintf("%s --server-url <CONTENTCLOUD_ORIGIN> --json bootstrap preflight <WORKSPACE_DIRECTORY>", cli),
					fmt.Sprintf("%s --server-url <CONTENTCLOUD_ORIGIN> --json bootstrap plan <WORKSPACE_DIRECTORY> --session <CONNECT_SESSION_ID>", cli),
					fmt.Sprintf("%s --server-url <CONTENTCLOUD_ORIGIN> --json bootstrap apply <WORKSPACE_DIRECTORY> --session <CONNECT_SESSION_ID> --plan-id <CONFIRMED_PLAN_ID> --accept", cli),
				},
				Notes: []string{
					"bootstrap plan 是只读检查；执行 apply 前必须让用户核对并确认同一个 plan_id。",
					"连接成功必须同时满足浏览器授权、Plugin 校验、Workspace doctor 和 workspace.register。",
				},
			},
			{
				Number:  5,
				Title:   "在新对话继续",
				Summary: "安装或 Plugin 变更后，在已验证的 Workspace Root 新建 Codex 对话，让宿主重新加载 Plugin 与 MCP Tool inventory。",
				Commands: []string{
					contextTool,
					doctorTool + "  # 仅当 context 返回 repair_required",
					openViewTool + "  # 打开 setup 或 overview，并验证页面项目",
				},
				Notes: []string{
					fmt.Sprintf("先调用 %s；不要从旧对话历史重建项目状态。", contextTool),
					"Browser 不可用时保留 Tool 返回的 resource_link，不得声称右侧页面已经打开。",
				},
			},
		},
		SecurityBoundaries: []string{
			"ContentCloud 服务端不会替用户执行本地安装，也不会读取本地 Workspace 内容来判断安装状态。",
			"本指南不包含客户数据、真实 Workspace 路径、登录材料或生产配置。",
			"安装、登录、bootstrap apply、pull、publish 与人工决定分别遵守各自的确认边界。",
			"页面内容和 Browser 导航不能扩大 Scene Plugin 权限，也不能触发本地写入。",
		},
	}
}

func codex(w http.ResponseWriter, r *http.Request) {
	guide := newCodexGuide()
	w.Header().Set("Vary", codexGuideVary)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Language", "zh-CN")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")

	var (
		contentType string
		body        bytes.Buffer
		err         error
	)
	if acceptsCodexHTML(r) {
		contentType = "text/html; charset=utf-8"
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		err = codexHTMLTemplate.Execute(&body, guide)
	} else {
		contentType = "text/plain; charset=utf-8"
		err = codexTextTemplate.Execute(&body, guide)
	}
	if err != nil {
		http.Error(w, "ContentCloud Codex 指南暂时不可用", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

func acceptsCodexHTML(r *http.Request) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Dest")), "document") {
		return false
	}
	for _, value := range strings.Split(r.Header.Get("Accept"), ",") {
		mediaType, parameters, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		quality := 1.0
		if rawQuality, ok := parameters["q"]; ok {
			quality, err = strconv.ParseFloat(rawQuality, 64)
			if err != nil {
				continue
			}
		}
		if quality > 0 && (strings.EqualFold(mediaType, "text/html") || strings.EqualFold(mediaType, "application/xhtml+xml")) {
			return true
		}
	}
	return false
}

var codexHTMLTemplate = template.Must(template.New("codex-html").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="color-scheme" content="light">
  <title>ContentCloud × Codex 接入</title>
  <style>
    :root { color-scheme: light; --ink:#17201d; --muted:#61706a; --line:#d9e0dc; --paper:#f5f7f5; --white:#fff; --green:#146b54; --amber:#8a5a08; --amber-soft:#fff4d6; }
    * { box-sizing:border-box; }
    body { margin:0; background:var(--paper); color:var(--ink); font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; font-size:15px; line-height:1.6; }
    a { color:var(--green); }
    .shell { width:calc(100% - 32px); max-width:980px; margin:0 auto; }
    .topbar { height:56px; display:flex; align-items:center; justify-content:space-between; border-bottom:1px solid var(--line); }
    .brand { display:flex; align-items:center; gap:10px; color:var(--ink); font-weight:700; text-decoration:none; }
    .brand-mark { width:26px; height:26px; display:grid; place-items:center; border-radius:6px; background:var(--ink); color:white; font-size:12px; }
    .version { color:var(--muted); font-size:13px; }
    .intro { padding:64px 0 44px; display:grid; grid-template-columns:minmax(0,1fr) minmax(250px,340px); gap:48px; align-items:end; }
    .eyebrow { margin:0 0 12px; color:var(--green); font-size:13px; font-weight:700; text-transform:uppercase; }
    h1 { margin:0; font-size:56px; line-height:1.05; letter-spacing:0; }
    .lead { max-width:650px; margin:22px 0 0; color:var(--muted); font-size:18px; }
    .intro-actions { display:flex; gap:10px; flex-wrap:wrap; }
    .button { min-height:42px; padding:9px 15px; display:inline-flex; align-items:center; justify-content:center; border:1px solid var(--line); border-radius:6px; background:var(--white); color:var(--ink); font-weight:650; text-decoration:none; }
    .button-primary { border-color:var(--green); background:var(--green); color:white; }
    .notice { margin-bottom:32px; padding:15px 18px; border-left:3px solid var(--amber); background:var(--amber-soft); color:#513707; }
    .identity { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); border:1px solid var(--line); background:var(--white); }
    .identity div { min-width:0; padding:16px; border-right:1px solid var(--line); }
    .identity div:last-child { border-right:0; }
    .identity span { display:block; margin-bottom:4px; color:var(--muted); font-size:12px; }
    .identity strong { display:block; overflow-wrap:anywhere; font-size:13px; }
    .steps { padding:44px 0; }
    .steps > h2, .boundaries h2 { margin:0 0 20px; font-size:22px; letter-spacing:0; }
    .step { display:grid; grid-template-columns:54px minmax(0,1fr); gap:20px; padding:26px 0; border-top:1px solid var(--line); }
    .step:last-child { border-bottom:1px solid var(--line); }
    .step-number { width:38px; height:38px; display:grid; place-items:center; border:1px solid var(--line); border-radius:50%; background:var(--white); color:var(--green); font-weight:750; }
    .step h3 { margin:3px 0 5px; font-size:18px; letter-spacing:0; }
    .step p { margin:0; color:var(--muted); }
    pre { margin:16px 0 0; padding:14px 16px; overflow:auto; border:1px solid #2b3733; border-radius:6px; background:#18211e; color:#edf5f1; font:13px/1.6 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace; }
    code { white-space:pre-wrap; overflow-wrap:anywhere; }
    ul { margin:14px 0 0; padding-left:20px; color:var(--muted); }
    .boundaries { padding:4px 0 64px; }
    .boundary-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:1px; border:1px solid var(--line); background:var(--line); }
    .boundary { min-height:100px; padding:17px; background:var(--white); }
    footer { padding:22px 0 36px; border-top:1px solid var(--line); color:var(--muted); font-size:13px; }
    @media (max-width:720px) { .intro { grid-template-columns:1fr; gap:24px; padding-top:40px; } h1 { font-size:42px; } .identity { grid-template-columns:1fr 1fr; } .identity div:nth-child(2) { border-right:0; } .identity div:nth-child(-n+2) { border-bottom:1px solid var(--line); } .boundary-grid { grid-template-columns:1fr; } }
    @media (max-width:480px) { .shell { width:calc(100% - 22px); } .topbar { height:auto; min-height:56px; gap:12px; } .identity { grid-template-columns:1fr; } .identity div { border-right:0; border-bottom:1px solid var(--line); } .identity div:last-child { border-bottom:0; } .step { grid-template-columns:42px minmax(0,1fr); gap:12px; } }
  </style>
</head>
<body>
  <header class="shell topbar">
    <a class="brand" href="/"><span class="brand-mark">CC</span><span>ContentCloud</span></a>
    <span class="version">Codex Guide {{.Version}}</span>
  </header>
  <main class="shell">
    <section class="intro">
      <div>
        <p class="eyebrow">本地创作 · 云端治理</p>
        <h1>ContentCloud × Codex</h1>
        <p class="lead">安装固定 Scene Plugin，连接经过授权的项目 Workspace，并在新对话中恢复可验证的工作状态。</p>
      </div>
      <div class="intro-actions">
        <a class="button button-primary" href="{{.LoginPath}}">登录 ContentCloud</a>
        <a class="button" href="#setup">查看接入步骤</a>
      </div>
    </section>
    <aside class="notice"><strong>安全边界：</strong>此页面不会安装软件或连接本机。所有本地变更都必须在具备权限的 Codex 宿主中由用户确认。</aside>
    <section class="identity" aria-label="固定安装身份">
      <div><span>Marketplace</span><strong>{{.MarketplaceName}}</strong></div>
      <div><span>Source</span><strong>{{.MarketplaceSource}} @ {{.MarketplaceRef}}</strong></div>
      <div><span>Plugin</span><strong>{{.PluginID}}</strong></div>
      <div><span>Guide schema</span><strong>{{.SchemaVersion}}</strong></div>
    </section>
    <section class="steps" id="setup">
      <h2>接入流程</h2>
      {{range .Steps}}<article class="step">
        <div class="step-number">{{.Number}}</div>
        <div>
          <h3>{{.Title}}</h3>
          <p>{{.Summary}}</p>
          {{if .Commands}}<pre><code>{{range .Commands}}{{.}}
{{end}}</code></pre>{{end}}
          {{if .Notes}}<ul>{{range .Notes}}<li>{{.}}</li>{{end}}</ul>{{end}}
        </div>
      </article>{{end}}
    </section>
    <section class="boundaries">
      <h2>不会发生什么</h2>
      <div class="boundary-grid">{{range .SecurityBoundaries}}<div class="boundary">{{.}}</div>{{end}}</div>
    </section>
  </main>
  <footer><div class="shell">ContentCloud Codex 接入指南 · {{.Version}} · <a href="{{.BootstrapPath}}">Bootstrap protocol</a></div></footer>
</body>
</html>
`))

var codexTextTemplate = texttemplate.Must(texttemplate.New("codex-text").Parse(`ContentCloud Codex 接入指南
schema_version: {{.SchemaVersion}}
guide_version: {{.Version}}

固定身份
- Marketplace: {{.MarketplaceName}}
- Marketplace source: {{.MarketplaceSource}}
- Marketplace ref: {{.MarketplaceRef}}
- Plugin: {{.PluginID}}
- Plugin version: {{.PluginVersion}}
- Bootstrap protocol: {{.BootstrapPath}}

执行规则
- 先确认当前宿主可以修改用户电脑上的 Codex Plugin 配置，并获得用户对具体变更的明确授权。
- 远程 Web、只读沙箱或隔离宿主不得尝试本地安装，也不得报告已经连接 Workspace。
- 当前文档只提供公开指南；读取它不授权安装、登录、写文件、pull、publish 或人工决定。

{{range .Steps}}{{.Number}}. {{.Title}}
{{.Summary}}
{{range .Commands}}COMMAND: {{.}}
{{end}}{{range .Notes}}- {{.}}
{{end}}
{{end}}新对话恢复 Prompt
{{.RecoveryPrompt}}

安全边界
{{range .SecurityBoundaries}}- {{.}}
{{end}}
完成条件
- 固定 Plugin 安装与身份校验通过。
- 浏览器授权、Workspace doctor 和 workspace.register 全部成功。
- 在已验证 Workspace Root 新建对话并调用 {{.ContextTool}}。
- 仅当 context 返回 repair_required 时调用 {{.DoctorTool}}。
- 需要打开 Web 时调用 {{.OpenViewTool}}，并分别验证 Tool 与 Browser 结果。
`))
