package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func digestPlan(plan Plan) (string, error) {
	unsigned := struct {
		SchemaVersion        string     `json:"schema_version"`
		Mode                 string     `json:"mode"`
		HostID               HostID     `json:"host_id"`
		Release              ReleaseRef `json:"release"`
		ObservedGeneration   string     `json:"observed_generation,omitempty"`
		State                Status     `json:"state"`
		Actions              []Action   `json:"actions"`
		BlockingReasons      []string   `json:"blocking_reasons,omitempty"`
		RequiresConfirmation bool       `json:"requires_confirmation"`
	}{plan.SchemaVersion, plan.Mode, plan.HostID, plan.Release, plan.ObservedGeneration, plan.State, plan.Actions, plan.BlockingReasons, plan.RequiresConfirmation}
	body, err := json.Marshal(unsigned)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
