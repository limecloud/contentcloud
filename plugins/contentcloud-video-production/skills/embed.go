package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed contentcloud-workspace contentcloud-marketing-video-script contentcloud-knowledge-extraction
var embedded embed.FS

const Workspace = "contentcloud-workspace"
const MarketingVideoScript = "contentcloud-marketing-video-script"
const KnowledgeExtraction = "contentcloud-knowledge-extraction"

func Names() []string { return []string{Workspace, KnowledgeExtraction, MarketingVideoScript} }

func Read(name, path string) ([]byte, error) {
	if name != Workspace && name != MarketingVideoScript && name != KnowledgeExtraction {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	clean := strings.TrimPrefix(path, "/")
	if clean == "" {
		clean = "SKILL.md"
	}
	if strings.Contains(clean, "..") {
		return nil, fmt.Errorf("invalid skill path")
	}
	return embedded.ReadFile(name + "/" + clean)
}

func Files(name string) ([]string, error) {
	if name != Workspace && name != MarketingVideoScript && name != KnowledgeExtraction {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	var out []string
	err := fs.WalkDir(embedded, name, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			out = append(out, strings.TrimPrefix(path, name+"/"))
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
