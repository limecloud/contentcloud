package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type DaemonRuntimePolicy struct {
	CurrentVersion  string `json:"current_version"`
	MinimumVersion  string `json:"minimum_version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateURL       string `json:"update_url,omitempty"`
	UpdateCommand   string `json:"update_command,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	UpdateRequired  bool   `json:"update_required"`
}

type DaemonPollResponse struct {
	Leased      bool                `json:"leased"`
	Lease       *Lease              `json:"lease,omitempty"`
	Runtime     DaemonRuntimePolicy `json:"runtime"`
	PollAfterMS int                 `json:"poll_after_ms"`
}

type daemonVersionPolicy struct {
	minimum   string
	latest    string
	updateURL string
}

func WithDaemonVersionPolicy(minimum, latest, updateURL string) Option {
	return func(service *Service) {
		service.daemonVersions = daemonVersionPolicy{minimum: strings.TrimSpace(minimum), latest: strings.TrimSpace(latest), updateURL: strings.TrimSpace(updateURL)}
	}
}

func (s *Service) PollDaemon(ctx context.Context, actor Actor, device domain.Device, caps []domain.Capability, claims []AutomationEnvironmentClaim, daemonVersion string) (DaemonPollResponse, error) {
	runtime := s.daemonRuntimePolicy(daemonVersion)
	if runtime.UpdateRequired {
		device.LastSeenAt = s.now().UTC()
		device.Capabilities = caps
		device.Version = strings.TrimSpace(daemonVersion)
		_ = s.store.SaveDevice(ctx, device)
		return DaemonPollResponse{Leased: false, Runtime: runtime, PollAfterMS: 60000}, nil
	}
	lease, err := s.PollWithRuntime(ctx, actor, device, caps, claims, daemonVersion)
	if err != nil {
		if isNotFound(err) {
			return DaemonPollResponse{Leased: false, Runtime: runtime, PollAfterMS: 5000}, nil
		}
		return DaemonPollResponse{}, err
	}
	return DaemonPollResponse{Leased: true, Lease: &lease, Runtime: runtime, PollAfterMS: 1000}, nil
}

func (s *Service) daemonRuntimePolicy(version string) DaemonRuntimePolicy {
	version = strings.TrimSpace(version)
	policy := DaemonRuntimePolicy{CurrentVersion: version, MinimumVersion: s.daemonVersions.minimum, LatestVersion: s.daemonVersions.latest, UpdateURL: s.daemonVersions.updateURL, UpdateAvailable: false, UpdateRequired: false}
	if policy.UpdateURL != "" {
		policy.UpdateCommand = "npx --yes @limecloud/contentcloud@latest update"
	}
	if policy.LatestVersion != "" && compareDaemonVersions(version, policy.LatestVersion) < 0 {
		policy.UpdateAvailable = true
	}
	if policy.MinimumVersion != "" && compareDaemonVersions(version, policy.MinimumVersion) < 0 {
		policy.UpdateRequired = true
		policy.UpdateAvailable = true
	}
	return policy
}

func compareDaemonVersions(left, right string) int {
	leftParts, leftPre := daemonVersionParts(left)
	rightParts, rightPre := daemonVersionParts(right)
	for index := 0; index < 3; index++ {
		if leftParts[index] < rightParts[index] {
			return -1
		}
		if leftParts[index] > rightParts[index] {
			return 1
		}
	}
	if leftPre == rightPre {
		return 0
	}
	if leftPre == "" {
		return 1
	}
	if rightPre == "" {
		return -1
	}
	if leftPre < rightPre {
		return -1
	}
	return 1
}

func daemonVersionParts(value string) ([3]int, string) {
	var parts [3]int
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	pieces := strings.SplitN(value, "-", 2)
	numbers := strings.Split(pieces[0], ".")
	for index := 0; index < len(numbers) && index < 3; index++ {
		parts[index], _ = strconv.Atoi(numbers[index])
	}
	pre := ""
	if len(pieces) == 2 {
		pre = pieces[1]
	}
	return parts, pre
}

func defaultPollDeadline(now time.Time, waitMS int) time.Time {
	if waitMS < 0 {
		waitMS = 0
	}
	if waitMS > 25000 {
		waitMS = 25000
	}
	return now.Add(time.Duration(waitMS) * time.Millisecond)
}
