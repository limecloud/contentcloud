package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed contentcloud-workspace contentcloud-marketing-video-script contentcloud-knowledge-extraction contentcloud-douyin-audience-strategy contentcloud-storyboard-production contentcloud-seedance-export contentcloud-seedance-execution
var embedded embed.FS

const Workspace = "contentcloud-workspace"
const MarketingVideoScript = "contentcloud-marketing-video-script"
const KnowledgeExtraction = "contentcloud-knowledge-extraction"
const DouyinAudienceStrategy = "contentcloud-douyin-audience-strategy"
const StoryboardProduction = "contentcloud-storyboard-production"
const SeedanceExport = "contentcloud-seedance-export"
const SeedanceExecution = "contentcloud-seedance-execution"

func Names() []string {
	return []string{Workspace, KnowledgeExtraction, MarketingVideoScript, DouyinAudienceStrategy, StoryboardProduction, SeedanceExport, SeedanceExecution}
}

func Read(name, path string) ([]byte, error) {
	if !validName(name) {
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
	if !validName(name) {
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

func validName(name string) bool {
	for _, candidate := range Names() {
		if name == candidate {
			return true
		}
	}
	return false
}
