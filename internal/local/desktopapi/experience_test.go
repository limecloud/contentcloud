package desktopapi

import (
	"testing"
	"time"

	localsync "github.com/limecloud/contentcloud/internal/local/sync"
)

func TestBuildExperienceProjectionKeepsBoundedEvidenceAndCountsOutcomes(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	events := make([]localsync.ProjectEvent, 0, 6)
	for index := 0; index < 4; index++ {
		events = append(events, localsync.ProjectEvent{
			ID: "event-failure-" + string(rune('a'+index)), ProjectID: "project-1", Cursor: uint64(index + 1),
			Type: "workspace.publish.failed", CreatedAt: now.Add(time.Duration(index) * time.Minute),
		})
	}
	events = append(events,
		localsync.ProjectEvent{ID: "event-retry", ProjectID: "project-1", Cursor: 5, Type: "workspace.publish.requeued", CreatedAt: now.Add(5 * time.Minute)},
		localsync.ProjectEvent{ID: "event-success", ProjectID: "project-1", Cursor: 6, Type: "workspace.publish.synced", CreatedAt: now.Add(6 * time.Minute)},
	)

	projection := buildExperienceProjection(events, now)
	if projection.SchemaVersion != ExperienceSchemaVersion || projection.EventCount != 6 || projection.FailureCount != 4 || projection.RecoveryCount != 1 || projection.SuccessCount != 1 {
		t.Fatalf("unexpected experience counts: %#v", projection)
	}
	if projection.PatternCount != 3 || len(projection.Patterns) != 3 {
		t.Fatalf("unexpected pattern count: %#v", projection.Patterns)
	}
	if projection.Patterns[0].Kind != "failure" || projection.Patterns[0].EvidenceCount != 4 || len(projection.Patterns[0].Evidence) != 3 {
		t.Fatalf("failure pattern evidence was not bounded: %#v", projection.Patterns[0])
	}
	if projection.LastUpdated == nil || !projection.LastUpdated.Equal(now.Add(6*time.Minute)) {
		t.Fatalf("last updated = %v", projection.LastUpdated)
	}
}

func TestBuildExperienceProjectionUsesNowWhenThereAreNoEvents(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	projection := buildExperienceProjection(nil, now)
	if projection.EventCount != 0 || projection.PatternCount != 0 || projection.LastUpdated == nil || !projection.LastUpdated.Equal(now) {
		t.Fatalf("empty projection = %#v", projection)
	}
}
