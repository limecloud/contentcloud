package skills

import (
	"strings"
	"testing"
)

func TestV5SkillsDeclareExecutionBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		required []string
	}{
		{DouyinAudienceStrategy, []string{"Codex local", "ContentCloud server", "Human", "publish", "pull approved", "candidate"}},
		{StoryboardProduction, []string{"Codex local", "ContentCloud server", "Human", "ApprovedSnapshot", "review_ready", "locked_digest"}},
		{SeedanceExport, []string{"Codex local", "ContentCloud server", "User in Seedance", "ApprovedSnapshot", "@图片N", "60-delivery"}},
		{SeedanceExecution, []string{"ContentCloud server", "Provider worker", "MediaGenerationJob", "awaiting_cost_approval", "modelark-mcp", "单镜头"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := readSkillFile(t, test.name, "SKILL.md")
			if strings.Contains(body, "TODO") {
				t.Fatal("Skill still contains TODO placeholders")
			}
			for _, required := range test.required {
				if !strings.Contains(body, required) {
					t.Fatalf("Skill is missing execution-boundary phrase %q", required)
				}
			}
		})
	}
}
