package projectview

import (
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/limecloud/contentcloud/contracts"
	"github.com/limecloud/contentcloud/internal/domain"
)

const SchemaVersion = "contentcloud.studio-surfaces/1.0"

var (
	idPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	surfaces      = mustLoadContract()
)

type FocusKindContract struct {
	Kind           string `json:"kind"`
	DigestRequired bool   `json:"digest_required,omitempty"`
	QueryKey       string `json:"query_key,omitempty"`
}

type PageContract struct {
	Route           string              `json:"route"`
	Label           string              `json:"label"`
	Eyebrow         string              `json:"eyebrow"`
	Title           string              `json:"title"`
	Description     string              `json:"description"`
	Section         *string             `json:"section"`
	SubmissionTypes []string            `json:"submission_types"`
	SnapshotTypes   []string            `json:"snapshot_types"`
	FocusKinds      []FocusKindContract `json:"focus_kinds"`
}

type Contract struct {
	SchemaVersion string                  `json:"schema_version"`
	Order         []string                `json:"order"`
	Views         map[string]PageContract `json:"views"`
}

type Focus = domain.ProjectNavigationFocus
type Target = domain.ProjectNavigation

type Link struct {
	URL         string `json:"url"`
	ProjectID   string `json:"project_id"`
	View        string `json:"view"`
	Focus       *Focus `json:"focus,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func IDs() []string {
	return append([]string(nil), surfaces.Order...)
}

func Page(view string) (PageContract, bool) {
	page, ok := surfaces.Views[view]
	return page, ok
}

func ValidateServerBase(value string) error {
	_, err := trustedServerBase(value)
	return err
}

func Build(serverBase, projectID string, target Target) (Link, error) {
	base, err := trustedServerBase(serverBase)
	if err != nil {
		return Link{}, err
	}
	if !idPattern.MatchString(projectID) {
		return Link{}, domain.Invalid("PROJECT_ID_INVALID", "工作区绑定中的 project_id 无效")
	}
	if err := Validate(target); err != nil {
		return Link{}, err
	}
	page := surfaces.Views[target.View]

	query := url.Values{"project": []string{projectID}}
	if target.Focus != nil {
		focusContract, _ := allowedFocus(page, target.Focus.Kind)
		if focusContract.QueryKey != "" {
			query.Set(focusContract.QueryKey, target.Focus.ID)
		} else {
			query.Set("focus_kind", target.Focus.Kind)
			query.Set("focus_id", target.Focus.ID)
		}
		if target.Focus.Digest != "" {
			query.Set("expected_digest", target.Focus.Digest)
		}
	}

	base.Path = "/" + strings.TrimPrefix(page.Route, "/")
	base.RawPath = ""
	base.RawQuery = query.Encode()
	return Link{
		URL:         base.String(),
		ProjectID:   projectID,
		View:        target.View,
		Focus:       target.Focus,
		Name:        "打开 Content Work OS " + page.Label,
		Description: page.Description,
	}, nil
}

func BuildStudioConnect(serverBase, sessionID string) (string, error) {
	base, err := trustedServerBase(serverBase)
	if err != nil {
		return "", err
	}
	if !idPattern.MatchString(sessionID) {
		return "", domain.Invalid("CONNECT_SESSION_ID_INVALID", "执行客户端连接会话标识无效")
	}
	base.Path = "/studio/connect"
	base.RawPath = ""
	base.RawQuery = url.Values{"session": []string{sessionID}}.Encode()
	return base.String(), nil
}

func Validate(target Target) error {
	page, ok := surfaces.Views[target.View]
	if !ok {
		return domain.Invalid("PROJECT_VIEW_INVALID", "不支持该 Content Work OS 项目视图")
	}
	if target.Focus != nil {
		focusContract, ok := allowedFocus(page, target.Focus.Kind)
		if !ok || !idPattern.MatchString(target.Focus.ID) {
			return domain.Invalid("PROJECT_FOCUS_INVALID", "项目页面焦点与视图不匹配或 ID 无效")
		}
		if target.Focus.Digest != "" && !digestPattern.MatchString(target.Focus.Digest) {
			return domain.Invalid("PROJECT_FOCUS_DIGEST_INVALID", "项目页面焦点摘要（digest）必须是完整的 sha256 摘要")
		}
		if focusContract.DigestRequired && target.Focus.Digest == "" {
			return domain.Invalid("PROJECT_FOCUS_DIGEST_REQUIRED", "该项目页面焦点需要不可变的版本摘要")
		}
	}
	return nil
}

func allowedFocus(page PageContract, kind string) (FocusKindContract, bool) {
	for _, focus := range page.FocusKinds {
		if focus.Kind == kind {
			return focus, true
		}
	}
	return FocusKindContract{}, false
}

func trustedServerBase(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, domain.Invalid("WEB_TARGET_UNTRUSTED", "工作区绑定中的 server_url 必须是可信的 Content Work OS 服务地址")
	}
	hostname := strings.ToLower(parsed.Hostname())
	secure := parsed.Scheme == "https"
	localHTTP := parsed.Scheme == "http" && (hostname == "localhost" || net.ParseIP(hostname) != nil && net.ParseIP(hostname).IsLoopback())
	if !secure && !localHTTP {
		return nil, domain.Invalid("WEB_TARGET_UNTRUSTED", "Content Work OS 工作台只允许使用 HTTPS；本机开发仅允许访问环回地址的 HTTP 服务")
	}
	parsed.Path = ""
	return parsed, nil
}

func mustLoadContract() Contract {
	var contract Contract
	if err := json.Unmarshal(contracts.StudioSurfacesV1Contract, &contract); err != nil {
		panic("invalid embedded Studio surface contract: " + err.Error())
	}
	if contract.SchemaVersion != SchemaVersion || len(contract.Order) == 0 || len(contract.Order) != len(contract.Views) {
		panic("invalid embedded Studio surface contract shape")
	}
	seen := map[string]bool{}
	for _, id := range contract.Order {
		page, ok := contract.Views[id]
		if !ok || seen[id] || page.Route == "" || page.Label == "" || page.Title == "" {
			panic("invalid embedded Studio surface contract entry")
		}
		seen[id] = true
	}
	return contract
}
