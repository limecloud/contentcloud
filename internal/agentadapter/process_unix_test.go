//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package agentadapter

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAgentCancellationKillsEntireProcessGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.CommandContext(ctx, "sh", "-c", `sleep 30 & child=$!; printf '%s' "$child" > "$1"; wait`, "sh", pidFile)
	configureAgentProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	childPID := waitForChildPID(t, pidFile)
	processGroupID := cmd.Process.Pid
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("agent parent did not exit after cancellation")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		groupErr := syscall.Kill(-processGroupID, 0)
		childErr := syscall.Kill(childPID, 0)
		if errors.Is(groupErr, syscall.ESRCH) && errors.Is(childErr, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("agent process group or child still exists after cancellation: pgid=%d child=%d", processGroupID, childPID)
}

func waitForChildPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(body)) != "" {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(body)))
			if parseErr != nil {
				t.Fatalf("parse child PID: %v", parseErr)
			}
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent child process did not start")
	return 0
}
