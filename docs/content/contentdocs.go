// Package contentdocs provides the embedded, public Content Work OS documentation catalog.
package contentdocs

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/limecloud/contentcloud/internal/agentadapter"
)

const SchemaVersion = "contentcloud.docs-catalog/1.0"

var ErrPageNotFound = errors.New("documentation page not found")

// Public files are allowlisted deliberately. The internal architecture directory
// is kept in the repository but never embedded into the public documentation service.
//
//go:embed README.md catalog.json getting-started.md concepts/*.md clients/*.md content-types/*.md guides/*/*.md troubleshooting/*.md
var publicFiles embed.FS

type Status string

const (
	StatusAvailable Status = "available"
	StatusLimited   Status = "limited"
	StatusPlanned   Status = "planned"
)

type PageSummary struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Status      Status `json:"status"`
}

type Page struct {
	PageSummary
	Markdown string `json:"markdown"`
}

type Section struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Pages       []PageSummary `json:"pages"`
}

type Capability struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type Client struct {
	ID           string       `json:"id"`
	DisplayName  string       `json:"display_name"`
	Status       Status       `json:"status"`
	Summary      string       `json:"summary"`
	PageSlug     string       `json:"page_slug"`
	Capabilities []Capability `json:"capabilities"`
}

type ContentType struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   Status `json:"status"`
	Summary  string `json:"summary"`
	PageSlug string `json:"page_slug"`
}

type Guide struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	ClientID      string `json:"client_id"`
	ContentTypeID string `json:"content_type_id"`
	Status        Status `json:"status"`
	Summary       string `json:"summary"`
	PageSlug      string `json:"page_slug"`
}

type Catalog struct {
	SchemaVersion string        `json:"schema_version"`
	Home          PageSummary   `json:"home"`
	Pages         []PageSummary `json:"pages"`
	Sections      []Section     `json:"sections"`
	Clients       []Client      `json:"clients"`
	ContentTypes  []ContentType `json:"content_types"`
	Guides        []Guide       `json:"guides"`
}

type sourcePage struct {
	PageSummary
	Source string `json:"source"`
}

type sourceSection struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	PageSlugs   []string `json:"page_slugs"`
}

type sourceCatalog struct {
	SchemaVersion string          `json:"schema_version"`
	Home          sourcePage      `json:"home"`
	Pages         []sourcePage    `json:"pages"`
	Sections      []sourceSection `json:"sections"`
	ContentTypes  []ContentType   `json:"content_types"`
	Guides        []Guide         `json:"guides"`
}

var (
	loadOnce       sync.Once
	loadedCatalog  sourceCatalog
	loadedPageByID map[string]sourcePage
	loadErr        error
	slugPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*(/[a-z0-9][a-z0-9-]*)*$`)
)

func LoadCatalog() (Catalog, error) {
	source, pageByID, err := load()
	if err != nil {
		return Catalog{}, err
	}
	pages := make([]PageSummary, 0, len(source.Pages)+len(agentadapter.Clients()))
	pages = append(pages, source.Home.PageSummary)
	for _, page := range source.Pages {
		pages = append(pages, mergeClientStatus(page.PageSummary))
	}

	clients := buildClients()
	for _, client := range clients {
		if _, exists := pageByID[client.PageSlug]; exists {
			continue
		}
		pages = append(pages, PageSummary{
			Slug: client.PageSlug, Title: client.DisplayName,
			Description: client.Summary, Kind: "client", Status: client.Status,
		})
	}
	sort.SliceStable(pages, func(left, right int) bool { return pages[left].Slug < pages[right].Slug })

	sections := make([]Section, 0, len(source.Sections))
	for _, section := range source.Sections {
		resolved := Section{ID: section.ID, Title: section.Title, Description: section.Description, Pages: make([]PageSummary, 0, len(section.PageSlugs))}
		for _, slug := range section.PageSlugs {
			resolved.Pages = append(resolved.Pages, pageByID[slug].PageSummary)
		}
		sections = append(sections, resolved)
	}

	return Catalog{
		SchemaVersion: source.SchemaVersion,
		Home:          source.Home.PageSummary,
		Pages:         pages,
		Sections:      sections,
		Clients:       clients,
		ContentTypes:  append([]ContentType(nil), source.ContentTypes...),
		Guides:        append([]Guide(nil), source.Guides...),
	}, nil
}

func LoadPage(slug string) (Page, error) {
	if !slugPattern.MatchString(slug) {
		return Page{}, ErrPageNotFound
	}
	_, pageByID, err := load()
	if err != nil {
		return Page{}, err
	}
	if source, ok := pageByID[slug]; ok {
		body, readErr := publicFiles.ReadFile(source.Source)
		if readErr != nil {
			return Page{}, fmt.Errorf("read documentation page %q: %w", slug, readErr)
		}
		return Page{PageSummary: mergeClientStatus(source.PageSummary), Markdown: string(body)}, nil
	}
	clientID, ok := strings.CutPrefix(slug, "clients/")
	if !ok || strings.Contains(clientID, "/") {
		return Page{}, ErrPageNotFound
	}
	client, known := agentadapter.Lookup(clientID)
	if !known || string(client.ID) != clientID {
		return Page{}, ErrPageNotFound
	}
	definition := buildClient(client)
	return Page{
		PageSummary: PageSummary{Slug: definition.PageSlug, Title: definition.DisplayName, Description: definition.Summary, Kind: "client", Status: definition.Status},
		Markdown:    generatedClientMarkdown(definition),
	}, nil
}

func load() (sourceCatalog, map[string]sourcePage, error) {
	loadOnce.Do(func() {
		body, err := publicFiles.ReadFile("catalog.json")
		if err != nil {
			loadErr = err
			return
		}
		if err := json.Unmarshal(body, &loadedCatalog); err != nil {
			loadErr = fmt.Errorf("decode documentation catalog: %w", err)
			return
		}
		loadedPageByID, loadErr = validateCatalog(loadedCatalog)
	})
	return loadedCatalog, loadedPageByID, loadErr
}

func validateCatalog(catalog sourceCatalog) (map[string]sourcePage, error) {
	if catalog.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported documentation catalog schema %q", catalog.SchemaVersion)
	}
	pages := make(map[string]sourcePage, len(catalog.Pages)+1)
	for _, page := range append([]sourcePage{catalog.Home}, catalog.Pages...) {
		if !slugPattern.MatchString(page.Slug) || page.Title == "" || page.Description == "" || page.Kind == "" || !validStatus(page.Status) {
			return nil, fmt.Errorf("invalid documentation page metadata for %q", page.Slug)
		}
		if _, exists := pages[page.Slug]; exists {
			return nil, fmt.Errorf("duplicate documentation page %q", page.Slug)
		}
		if !validSourcePath(page.Source) {
			return nil, fmt.Errorf("unsafe documentation source %q", page.Source)
		}
		if _, err := fs.Stat(publicFiles, page.Source); err != nil {
			return nil, fmt.Errorf("missing documentation source %q: %w", page.Source, err)
		}
		if page.Kind == "client" {
			clientID, ok := strings.CutPrefix(page.Slug, "clients/")
			definition, known := agentadapter.Lookup(clientID)
			if !ok || !known || string(definition.ID) != clientID {
				return nil, fmt.Errorf("client page %q does not match the Agent Client Registry", page.Slug)
			}
		}
		pages[page.Slug] = page
	}
	for _, section := range catalog.Sections {
		if section.ID == "" || section.Title == "" || len(section.PageSlugs) == 0 {
			return nil, fmt.Errorf("invalid documentation section %q", section.ID)
		}
		for _, slug := range section.PageSlugs {
			if _, exists := pages[slug]; !exists {
				return nil, fmt.Errorf("section %q references unknown page %q", section.ID, slug)
			}
		}
	}
	contentStatuses := make(map[string]Status, len(catalog.ContentTypes))
	for _, contentType := range catalog.ContentTypes {
		if contentType.ID == "" || !validStatus(contentType.Status) || pages[contentType.PageSlug].Kind != "content_type" {
			return nil, fmt.Errorf("invalid content type documentation %q", contentType.ID)
		}
		if _, exists := contentStatuses[contentType.ID]; exists {
			return nil, fmt.Errorf("duplicate content type documentation %q", contentType.ID)
		}
		contentStatuses[contentType.ID] = contentType.Status
	}
	guideIDs := make(map[string]struct{}, len(catalog.Guides))
	for _, guide := range catalog.Guides {
		if guide.ID == "" || guide.ClientID == "" || guide.ContentTypeID == "" || !validStatus(guide.Status) || pages[guide.PageSlug].Kind != "guide" {
			return nil, fmt.Errorf("invalid guide documentation %q", guide.ID)
		}
		definition, known := agentadapter.Lookup(guide.ClientID)
		if !known || string(definition.ID) != guide.ClientID {
			return nil, fmt.Errorf("guide %q references unknown client %q", guide.ID, guide.ClientID)
		}
		if _, exists := guideIDs[guide.ID]; exists {
			return nil, fmt.Errorf("duplicate guide documentation %q", guide.ID)
		}
		guideIDs[guide.ID] = struct{}{}
		if guide.Status != StatusAvailable || buildClient(definition).Status != StatusAvailable || contentStatuses[guide.ContentTypeID] != StatusAvailable {
			return nil, fmt.Errorf("guide %q must reference an available client and content type", guide.ID)
		}
	}
	return pages, nil
}

func validSourcePath(value string) bool {
	return value != "" && path.Clean(value) == value && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "internal/") && strings.HasSuffix(value, ".md")
}

func validStatus(value Status) bool {
	return value == StatusAvailable || value == StatusLimited || value == StatusPlanned
}

func buildClients() []Client {
	definitions := agentadapter.Clients()
	clients := make([]Client, 0, len(definitions))
	for _, definition := range definitions {
		clients = append(clients, buildClient(definition))
	}
	return clients
}

func buildClient(definition agentadapter.ClientDefinition) Client {
	available := 0
	capabilities := make([]Capability, 0, len(definition.Capabilities))
	for _, capability := range definition.Capabilities {
		if capability.Status == agentadapter.SupportAvailable {
			available++
		}
		capabilities = append(capabilities, Capability{ID: string(capability.ID), Status: string(capability.Status)})
	}
	status := StatusPlanned
	if available == len(capabilities) {
		status = StatusAvailable
	} else if available > 0 {
		status = StatusLimited
	}
	return Client{
		ID: string(definition.ID), DisplayName: definition.DisplayName, Status: status,
		Summary: clientSummary(definition.DisplayName, status), PageSlug: "clients/" + string(definition.ID), Capabilities: capabilities,
	}
}

func clientSummary(displayName string, status Status) string {
	switch status {
	case StatusAvailable:
		return displayName + " 已支持完整的 Content Work OS 交互式工作流。"
	case StatusLimited:
		return displayName + " 已开放部分底层能力，完整交互式工作流仍在接入。"
	default:
		return displayName + " 已进入兼容目录，具体能力即将支持。"
	}
}

func mergeClientStatus(page PageSummary) PageSummary {
	if page.Kind != "client" {
		return page
	}
	clientID, ok := strings.CutPrefix(page.Slug, "clients/")
	if !ok {
		return page
	}
	definition, known := agentadapter.Lookup(clientID)
	if !known || string(definition.ID) != clientID {
		return page
	}
	page.Status = buildClient(definition).Status
	return page
}

func generatedClientMarkdown(client Client) string {
	var body strings.Builder
	fmt.Fprintf(&body, "# Content Work OS 与 %s\n\n状态：**%s**。\n\n", client.DisplayName, statusLabel(client.Status))
	fmt.Fprintf(&body, "Content Work OS 已为 %s 提供稳定的客户端标识、能力目录和文档入口。本页由客户端能力目录自动生成，只说明已经登记的能力状态；在正式教程发布前，不提供安装、工作区初始化、任务交接或内容发布命令。\n\n", client.DisplayName)
	body.WriteString("## 能力状态\n\n| 能力 | 状态 |\n| --- | --- |\n")
	for _, capability := range client.Capabilities {
		fmt.Fprintf(&body, "| %s | %s |\n", capabilityLabel(capability.ID), supportStatusLabel(capability.Status))
	}
	body.WriteString("\n## 现在可以做什么\n\n- 在 Content Work OS 工作台中继续项目治理和审核。\n- 使用当前标记为可用的客户端与内容形态场景。\n- 以后继续使用同一个文档地址查看能力状态，无需寻找新的入口。\n\n规划状态不代表相关命令、协议或第三方集成已经可用。\n")
	return body.String()
}

func statusLabel(status Status) string {
	switch status {
	case StatusAvailable:
		return "可用"
	case StatusLimited:
		return "有限支持"
	default:
		return "即将支持"
	}
}

func supportStatusLabel(status string) string {
	if status == string(agentadapter.SupportAvailable) {
		return "可用"
	}
	return "即将支持"
}

func capabilityLabel(id string) string {
	switch id {
	case string(agentadapter.CapabilityLocalAutomation):
		return "本地自动化"
	case string(agentadapter.CapabilityWorkspaceRegister):
		return "工作区注册"
	case string(agentadapter.CapabilityWorkspaceBootstrap):
		return "工作区初始化"
	case string(agentadapter.CapabilityInteractiveHandoff):
		return "交互式任务交接"
	case string(agentadapter.CapabilityCreativeEnvironment):
		return "创作环境"
	default:
		return id
	}
}
