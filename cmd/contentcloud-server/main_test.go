package main

import "testing"

func TestRuntimeRolloutDefaultsClosedAndRequiresExplicitEnablement(t *testing.T) {
	t.Setenv("CONTENTCLOUD_RUNTIME_ADMISSION_ENABLED", "")
	t.Setenv("CONTENTCLOUD_RUNTIME_DYNAMIC_GRAPH_ENABLED", "")
	t.Setenv("CONTENTCLOUD_RUNTIME_CANARY_TENANT_IDS", "")
	policy, err := runtimeRolloutFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if policy.AdmissionEnabled || policy.DynamicGraphEnabled || len(policy.TenantIDs) != 0 {
		t.Fatalf("unset production rollout must fail closed: %#v", policy)
	}

	t.Setenv("CONTENTCLOUD_RUNTIME_ADMISSION_ENABLED", "1")
	t.Setenv("CONTENTCLOUD_RUNTIME_DYNAMIC_GRAPH_ENABLED", "true")
	t.Setenv("CONTENTCLOUD_RUNTIME_CANARY_TENANT_IDS", "tenant-a, tenant-b")
	policy, err = runtimeRolloutFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !policy.AdmissionEnabled || !policy.DynamicGraphEnabled || len(policy.TenantIDs) != 2 || policy.TenantIDs[0] != "tenant-a" || policy.TenantIDs[1] != "tenant-b" {
		t.Fatalf("explicit Canary rollout was not preserved: %#v", policy)
	}
}

func TestRuntimeRolloutRejectsInvalidBoolean(t *testing.T) {
	t.Setenv("CONTENTCLOUD_RUNTIME_ADMISSION_ENABLED", "enabled")
	t.Setenv("CONTENTCLOUD_RUNTIME_DYNAMIC_GRAPH_ENABLED", "0")
	if _, err := runtimeRolloutFromEnv(); err == nil {
		t.Fatal("invalid Runtime rollout boolean was accepted")
	}
}
