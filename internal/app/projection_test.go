package app

import (
	"context"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestProjectionNextActionsUseTypedPageTargets(t *testing.T) {
	tests := []struct {
		name     string
		project  domain.Project
		sections map[string]domain.ProjectionSection
		wantView string
		enabled  bool
	}{
		{name: "archived", project: domain.Project{Status: "archived"}, sections: map[string]domain.ProjectionSection{}, wantView: "overview", enabled: false},
		{name: "onboarding", project: domain.Project{Status: "active"}, sections: map[string]domain.ProjectionSection{}, wantView: "setup", enabled: true},
		{name: "knowledge", project: domain.Project{Status: "active"}, sections: map[string]domain.ProjectionSection{"onboarding": {Count: 1}}, wantView: "knowledge", enabled: true},
		{name: "assignment", project: domain.Project{Status: "active"}, sections: map[string]domain.ProjectionSection{"onboarding": {Count: 1}, "knowledge": {Count: 1}}, wantView: "planning", enabled: true},
	}
	service := &Service{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actions, err := service.projectionNextActions(context.Background(), "tenant-1", test.project, test.sections, domain.ProjectionGovernance{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(actions) != 1 || actions[0].Navigation.View != test.wantView || actions[0].Enabled != test.enabled {
				t.Fatalf("unexpected next action: %#v", actions)
			}
		})
	}
}
