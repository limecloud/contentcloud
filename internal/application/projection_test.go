package application

import (
	"context"
	"testing"

	"github.com/limecloud/contentcloud/internal/experience/projection"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestProjectionNextActionsUseTypedPageTargets(t *testing.T) {
	tests := []struct {
		name     string
		project  workspacedomain.Project
		sections map[string]projection.ProjectionSection
		wantView string
		enabled  bool
	}{
		{name: "archived", project: workspacedomain.Project{Status: "archived"}, sections: map[string]projection.ProjectionSection{}, wantView: "home", enabled: false},
		{name: "onboarding", project: workspacedomain.Project{Status: "active"}, sections: map[string]projection.ProjectionSection{}, wantView: "connect", enabled: true},
		{name: "knowledge", project: workspacedomain.Project{Status: "active"}, sections: map[string]projection.ProjectionSection{"onboarding": {Count: 1}}, wantView: "knowledge", enabled: true},
		{name: "assignment", project: workspacedomain.Project{Status: "active"}, sections: map[string]projection.ProjectionSection{"onboarding": {Count: 1}, "knowledge": {Count: 1}}, wantView: "tasks", enabled: true},
	}
	service := &OperationsService{serviceScope: &serviceScope{serviceCore: &serviceCore{}}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actions, err := service.projectionNextActions(context.Background(), "tenant-1", test.project, test.sections, projection.ProjectionGovernance{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(actions) != 1 || actions[0].Navigation.View != test.wantView || actions[0].Enabled != test.enabled {
				t.Fatalf("unexpected next action: %#v", actions)
			}
		})
	}
}
