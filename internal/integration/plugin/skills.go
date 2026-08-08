package plugin

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func discoverSkills(root string, limits Limits) ([]Skill, []Diagnostic) {
	directory := filepath.Join(root, "skills")
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return []Skill{}, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return []Skill{}, []Diagnostic{{Level: DiagnosticError, Code: "PLUGIN_SKILLS_LOCATION_INVALID", Path: "skills", Message: "skills must be a non-symlink directory"}}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return []Skill{}, []Diagnostic{{Level: DiagnosticError, Code: "PLUGIN_SKILLS_READ_FAILED", Path: "skills", Message: err.Error()}}
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	skills := make([]Skill, 0)
	diagnostics := make([]Diagnostic, 0)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		relative := filepath.ToSlash(filepath.Join("skills", name, "SKILL.md"))
		body, err := readPackageFile(root, relative, limits.MaxFileBytes)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Level: DiagnosticError, Code: "PLUGIN_SKILL_FILE_INVALID", Path: relative, Component: name, Message: err.Error()})
			continue
		}
		frontmatter, err := parseSkillFrontmatter(body, name)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Level: DiagnosticError, Code: "PLUGIN_SKILL_INVALID", Path: relative, Component: name, Message: err.Error()})
			continue
		}
		skills = append(skills, Skill{Name: frontmatter.Name, Description: frontmatter.Description, Path: relative, Digest: bytesDigest(body)})
	}
	return skills, diagnostics
}

func parseSkillFrontmatter(body []byte, directoryName string) (skillFrontmatter, error) {
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	if !bytes.HasPrefix(body, []byte("---\n")) {
		return skillFrontmatter{}, fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}
	end := bytes.Index(body[4:], []byte("\n---"))
	if end < 0 {
		return skillFrontmatter{}, fmt.Errorf("SKILL.md frontmatter is not closed")
	}
	frontmatterBody := body[4 : 4+end]
	var frontmatter skillFrontmatter
	decoder := yaml.NewDecoder(bytes.NewReader(frontmatterBody))
	if err := decoder.Decode(&frontmatter); err != nil {
		return skillFrontmatter{}, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	if frontmatter.Name != directoryName || !validSkillName(frontmatter.Name) {
		return skillFrontmatter{}, fmt.Errorf("skill name %q must match directory %q and use lowercase alphanumerics with single hyphens", frontmatter.Name, directoryName)
	}
	if strings.TrimSpace(frontmatter.Description) == "" || utf8.RuneCountInString(frontmatter.Description) > 1024 {
		return skillFrontmatter{}, fmt.Errorf("skill description must contain 1-1024 characters")
	}
	return frontmatter, nil
}

func validSkillName(value string) bool {
	return len(value) <= 64 && skillNamePattern.MatchString(value)
}
