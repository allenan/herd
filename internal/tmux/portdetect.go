package tmux

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DetectListeningPort checks whether any descendant process of the given
// tmux pane is listening on a TCP port. Returns the lowest port number
// found, or 0 if none.
func DetectListeningPort(paneID string) int {
	// Get the pane's PID
	pidStr, err := TmuxRunOutput("display-message", "-p", "-t", paneID, "#{pane_pid}")
	if err != nil {
		return 0
	}
	pidStr = strings.TrimSpace(pidStr)
	rootPID, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0
	}

	// Collect all descendant PIDs (depth-limited)
	pids := collectDescendants(rootPID, 5)
	pids = append(pids, rootPID)

	return findListeningPort(pids)
}

// collectDescendants walks the process tree from rootPID down to maxDepth
// levels, returning all descendant PIDs found.
func collectDescendants(rootPID, maxDepth int) []int {
	if maxDepth <= 0 {
		return nil
	}

	out, err := exec.Command("pgrep", "-P", strconv.Itoa(rootPID)).Output()
	if err != nil {
		return nil
	}

	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		pids = append(pids, pid)
		pids = append(pids, collectDescendants(pid, maxDepth-1)...)
	}
	return pids
}

// findListeningPort runs lsof to find TCP LISTEN sockets for any of the
// given PIDs. Returns the lowest port number found, or 0 if none.
func findListeningPort(pids []int) int {
	if len(pids) == 0 {
		return 0
	}

	// Build a comma-separated PID list for lsof
	pidStrs := make([]string, len(pids))
	for i, p := range pids {
		pidStrs[i] = strconv.Itoa(p)
	}

	// lsof -nP -iTCP -sTCP:LISTEN -a -p <pids>
	// -a means AND the filters (TCP + LISTEN + these PIDs)
	out, err := exec.Command(
		"lsof", "-nP", "-iTCP", "-sTCP:LISTEN",
		"-a", "-p", strings.Join(pidStrs, ","),
	).Output()
	if err != nil {
		return 0
	}

	lowestPort := 0
	for _, line := range strings.Split(string(out), "\n") {
		port := parseLsofPort(line)
		if port > 0 && (lowestPort == 0 || port < lowestPort) {
			lowestPort = port
		}
	}
	return lowestPort
}

// parseLsofPort extracts the port number from an lsof output line.
// Example line: "node    12345 user   20u  IPv4 0x... TCP *:3000 (LISTEN)"
// The name field contains something like "*:3000" or "127.0.0.1:8080".
func parseLsofPort(line string) int {
	// Look for the TCP field pattern: <addr>:<port>
	// It appears before "(LISTEN)" in the line
	idx := strings.Index(line, "(LISTEN)")
	if idx < 0 {
		return 0
	}
	// Work backwards from "(LISTEN)" to find the port
	before := strings.TrimSpace(line[:idx])
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return 0
	}
	addrPort := fields[len(fields)-1]
	colonIdx := strings.LastIndex(addrPort, ":")
	if colonIdx < 0 {
		return 0
	}
	port, err := strconv.Atoi(addrPort[colonIdx+1:])
	if err != nil {
		return 0
	}
	// Sanity check
	if port < 1 || port > 65535 {
		return 0
	}
	// Skip ephemeral ports (likely not user services)
	if port > 49151 {
		return 0
	}
	return port
}

// PanePID returns the PID of the process running in a tmux pane.
func PanePID(paneID string) (int, error) {
	pidStr, err := TmuxRunOutput("display-message", "-p", "-t", paneID, "#{pane_pid}")
	if err != nil {
		return 0, err
	}
	pidStr = strings.TrimSpace(pidStr)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid pane pid %q: %w", pidStr, err)
	}
	return pid, nil
}
