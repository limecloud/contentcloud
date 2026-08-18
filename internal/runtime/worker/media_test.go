package worker

import (
	"context"
	"errors"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"reflect"
	"testing"
)

func TestProcessPendingMediaProcessesJobsInOrder(t *testing.T) {
	processor := &mediaProcessorFixture{jobs: []deliverydomain.MediaGenerationJob{
		{ID: "job-1", TenantID: "tenant-1"},
		{ID: "job-2", TenantID: "tenant-2"},
	}}

	processed, err := ProcessPendingMedia(t.Context(), processor, 5)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 2 || processor.limit != 5 || !reflect.DeepEqual(processor.calls, []string{"tenant-1/job-1", "tenant-2/job-2"}) {
		t.Fatalf("unexpected media processing result: processed=%d limit=%d calls=%#v", processed, processor.limit, processor.calls)
	}
}

func TestProcessPendingMediaStopsAfterProcessingError(t *testing.T) {
	processor := &mediaProcessorFixture{jobs: []deliverydomain.MediaGenerationJob{
		{ID: "job-1", TenantID: "tenant-1"},
		{ID: "job-2", TenantID: "tenant-1"},
		{ID: "job-3", TenantID: "tenant-1"},
	}, failJobID: "job-2"}

	processed, err := ProcessPendingMedia(t.Context(), processor, 3)
	if !errors.Is(err, errMediaFixture) {
		t.Fatalf("error = %v, want fixture error", err)
	}
	if processed != 1 || !reflect.DeepEqual(processor.calls, []string{"tenant-1/job-1", "tenant-1/job-2"}) {
		t.Fatalf("worker continued after error: processed=%d calls=%#v", processed, processor.calls)
	}
}

var errMediaFixture = errors.New("media fixture failed")

type mediaProcessorFixture struct {
	jobs      []deliverydomain.MediaGenerationJob
	failJobID string
	limit     int
	calls     []string
}

func (f *mediaProcessorFixture) PendingMediaGenerationJobs(_ context.Context, limit int) ([]deliverydomain.MediaGenerationJob, error) {
	f.limit = limit
	return append([]deliverydomain.MediaGenerationJob(nil), f.jobs...), nil
}

func (f *mediaProcessorFixture) ProcessMediaGenerationJob(_ context.Context, tenantID, id string) error {
	f.calls = append(f.calls, tenantID+"/"+id)
	if id == f.failJobID {
		return errMediaFixture
	}
	return nil
}
