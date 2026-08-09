package agentadapter

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestFakeHarnessIsDeterministicAndResumable(t *testing.T) {
	harness := NewFakeHarness()
	capabilities, err := harness.Detect(t.Context())
	if err != nil || !capabilities.Resume || !capabilities.Events || capabilities.Kind != "fake" {
		t.Fatalf("unexpected fake capabilities: %#v err=%v", capabilities, err)
	}
	ref, stream, err := harness.Start(t.Context(), StartAgentRequest{JobRunID: "job-1", NodeRunID: "node-1", AttemptID: "attempt-1"})
	if err != nil {
		t.Fatal(err)
	}
	started := <-stream.Events()
	if started.Type != "session.started" || started.Session != ref {
		t.Fatalf("unexpected start event: %#v", started)
	}
	resumed, err := harness.Resume(t.Context(), ResumeAgentRequest{Session: ref})
	if err != nil {
		t.Fatal(err)
	}
	if event := <-resumed.Events(); event.Type != "session.resumed" {
		t.Fatalf("unexpected resume event: %#v", event)
	}
	if err := harness.Complete(ref, map[string]string{"status": "ok"}); err != nil {
		t.Fatal(err)
	}
	completed := <-stream.Events()
	if completed.Type != "result.completed" || string(completed.Data) != `{"status":"ok"}` {
		t.Fatalf("unexpected completion event: %#v", completed)
	}
	if _, open := <-stream.Events(); open {
		t.Fatal("fake stream did not close after completion")
	}
	status, err := harness.Inspect(t.Context(), ref)
	if err != nil || status.State != "completed" {
		t.Fatalf("unexpected final status: %#v err=%v", status, err)
	}
}

func TestHarnessRegistryReusesAdapterAndSessionState(t *testing.T) {
	registry := NewHarnessRegistry()
	fake := NewFakeHarness()
	if err := registry.Register("fake", fake); err != nil {
		t.Fatal(err)
	}
	first, capabilities, err := registry.Resolve(t.Context(), "fakeharness")
	if err != nil {
		t.Fatal(err)
	}
	if capabilities.Kind != "fake" || !capabilities.Resume {
		t.Fatalf("unexpected capabilities: %#v", capabilities)
	}
	ref, _, err := first.Start(t.Context(), StartAgentRequest{NodeRunID: "node-1", AttemptID: "attempt-1"})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := registry.Resolve(t.Context(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("registry replaced a long-lived harness adapter")
	}
	if _, err := second.Resume(t.Context(), ResumeAgentRequest{Session: ref}); err != nil {
		t.Fatalf("resolved adapter lost session state: %v", err)
	}
}

func TestFakeHarnessRunsQueuedScriptAndCloses(t *testing.T) {
	fake := NewFakeHarness()
	result := json.RawMessage(`{"output_refs":["asset:1"],"output_digest":"sha256:result"}`)
	fake.QueueScript(FakeHarnessScript{Events: []FakeHarnessScriptEvent{
		{Type: "progress", Data: json.RawMessage(`{"percent":50}`)},
		{Type: "result.completed", Data: result},
		{Type: "result.completed", Data: result},
	}})
	_, stream, err := fake.Start(t.Context(), StartAgentRequest{NodeRunID: "node-1", AttemptID: "attempt-1"})
	if err != nil {
		t.Fatal(err)
	}
	types := []string{}
	for event := range stream.Events() {
		types = append(types, event.Type)
	}
	if len(types) != 4 || types[0] != "session.started" || types[3] != "result.completed" {
		t.Fatalf("unexpected scripted events: %#v", types)
	}
}

func containsDomainCode(err error, code string) bool {
	var domainErr *domain.Error
	return errors.As(err, &domainErr) && domainErr.Code == code
}
