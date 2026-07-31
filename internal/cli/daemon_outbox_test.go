package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func TestDaemonJournalPersistsReportBeforeNetworkDeliveryAndFlushesAfterRestart(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CONTENTCLOUD_DAEMON_STATE_DIR", stateDir)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	journal, err := newDaemonJournal()
	if err != nil {
		t.Fatal(err)
	}
	journal.now = func() time.Time { return now }
	lease := app.Lease{Run: domain.TaskRun{ID: "run-1"}, Attempt: domain.RunAttempt{ID: "attempt-1"}, RunToken: "rt_secret"}
	if err := journal.begin(lease, "http://server", "device-1"); err != nil {
		t.Fatal(err)
	}
	packageBody := json.RawMessage(`{"schema_version":"1.0","candidates":[{}]}`)
	if err := journal.queueReport(lease, packageBody); err != nil {
		t.Fatal(err)
	}
	unavailable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unavailableURL := unavailable.URL
	unavailable.Close()
	failedClient := apiclient.New(unavailableURL, "device-token")
	failedClient.HTTP.Timeout = 200 * time.Millisecond
	if err := journal.flush(context.Background(), failedClient); err == nil {
		t.Fatal("network failure did not preserve the pending report")
	}
	pending, err := os.ReadDir(filepath.Join(stateDir, "outbox"))
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending report was not retained: entries=%#v err=%v", pending, err)
	}

	restarted, err := newDaemonJournal()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Command string         `json:"command"`
			Params  map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Command != "run.report" || request.Params["run_token"] != "rt_secret" {
			t.Fatalf("unexpected journal dispatch: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"command":"run.report","data":{},"meta":{}}`))
	}))
	defer server.Close()
	client := apiclient.New(server.URL, "device-token")
	client.HTTP.Timeout = time.Second
	if err := restarted.flush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(stateDir, "outbox"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("journal was not acknowledged: entries=%#v err=%v", entries, err)
	}
}

func TestDaemonJournalMovesPermanentDeliveryFailureToDeadLetter(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CONTENTCLOUD_DAEMON_STATE_DIR", stateDir)
	journal, err := newDaemonJournal()
	if err != nil {
		t.Fatal(err)
	}
	lease := app.Lease{Run: domain.TaskRun{ID: "run-1"}, Attempt: domain.RunAttempt{ID: "attempt-1"}, RunToken: "rt_secret"}
	if err := journal.begin(lease, "http://server", "device-1"); err != nil {
		t.Fatal(err)
	}
	if err := journal.queueReport(lease, json.RawMessage(`{"schema_version":"1.0"}`)); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"command":"run.report","error":{"type":"conflict","code":"RUN_ATTEMPT_STALE","message":"stale","retryable":false}}`))
	}))
	defer server.Close()
	if err := journal.flush(context.Background(), apiclient.New(server.URL, "device-token")); err == nil {
		t.Fatal("permanent delivery failure was swallowed")
	}
	entries, err := os.ReadDir(filepath.Join(stateDir, "outbox"))
	if err != nil {
		t.Fatal(err)
	}
	foundDead := false
	for _, entry := range entries {
		foundDead = foundDead || filepath.Ext(entry.Name()) == ".dead"
	}
	if !foundDead {
		t.Fatalf("permanent failure did not leave a dead-letter marker: %#v", entries)
	}
	pending, dead, err := daemonJournalCounts()
	if err != nil || pending != 0 || dead != 1 {
		t.Fatalf("journal counts after permanent failure: pending=%d dead=%d err=%v", pending, dead, err)
	}
	service := &launchdDaemonService{home: t.TempDir(), uid: 501, run: func(string, ...string) ([]byte, error) { return nil, domain.NotFound("daemon") }}
	status, err := service.Status()
	if err != nil || status.DeadLetters != 1 || status.PendingReports != 0 {
		t.Fatalf("daemon status did not expose journal counts: status=%#v err=%v", status, err)
	}
}

func TestDaemonJournalFinishesInterruptedAttemptAfterRestart(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CONTENTCLOUD_DAEMON_STATE_DIR", stateDir)
	journal, err := newDaemonJournal()
	if err != nil {
		t.Fatal(err)
	}
	lease := app.Lease{Run: domain.TaskRun{ID: "run-interrupted"}, Attempt: domain.RunAttempt{ID: "attempt-interrupted"}, RunToken: "rt_interrupted"}
	if err := journal.begin(lease, "http://server", "device-1"); err != nil {
		t.Fatal(err)
	}

	restarted, err := newDaemonJournal()
	if err != nil {
		t.Fatal(err)
	}
	finished := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Command string         `json:"command"`
			Params  map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Command != "run.finish" || request.Params["outcome"] != "failed" || request.Params["failure_class"] != "daemon_restarted" {
			t.Fatalf("unexpected interrupted attempt recovery: %#v", request)
		}
		finished = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"command":"run.finish","data":{},"meta":{}}`))
	}))
	defer server.Close()
	if err := restarted.flush(context.Background(), apiclient.New(server.URL, "device-token")); err != nil {
		t.Fatal(err)
	}
	if !finished {
		t.Fatal("interrupted attempt was not finished after restart")
	}
}
