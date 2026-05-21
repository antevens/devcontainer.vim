package docker

import (
	"os/exec"
	"strings"
	"testing"
)

func requireDockerPsJSONSupport(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker command not found, skipping test")
	}

	if err := exec.Command("docker", "ps", "--format", "json").Run(); err != nil {
		t.Skipf("docker ps --format json not available in this environment: %v", err)
	}
}

func TestPs(t *testing.T) {
	requireDockerPsJSONSupport(t)

	// Test that the basic docker ps command can be executed without error
	result, err := Ps("status=running")
	if err != nil {
		// Skip if Docker daemon is not running, etc.
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
			t.Skip("Docker daemon not running, skipping test")
		}
		t.Fatalf("Ps failed: %v", err)
	}

	// Confirm that the result is a string
	if result == "" {
		// Returns an empty string if there are no running containers
		t.Logf("No running containers found")
	} else {
		// Check if the result contains JSON format (simple)
		if !strings.Contains(result, "ID") {
			t.Logf("Unexpected output format: %s", result)
		}
	}
}

func TestPsWithInvalidFilter(t *testing.T) {
	requireDockerPsJSONSupport(t)

	// Test with invalid filter
	result, err := Ps("invalid=filter")
	if err != nil {
		// Skip if Docker daemon is not running, etc.
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
			t.Skip("Docker daemon not running, skipping test")
		}
		// Even with an invalid filter, the docker command itself may succeed
		t.Logf("Ps with invalid filter returned error: %v", err)
	}

	// Result is expected to be an empty string
	if result != "" {
		t.Logf("Unexpected result with invalid filter: %s", result)
	}
}

func TestPsWithEmptyFilter(t *testing.T) {
	requireDockerPsJSONSupport(t)

	// Test with empty filter
	result, err := Ps("")
	if err != nil {
		// Skip if Docker daemon is not running, etc.
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
			t.Skip("Docker daemon not running, skipping test")
		}
		t.Fatalf("Ps with empty filter failed: %v", err)
	}

	// Confirm that the result is a string (can be empty)
	t.Logf("Ps with empty filter result: %s", result)
}

func TestPsWithValidLabel(t *testing.T) {
	requireDockerPsJSONSupport(t)

	// Test with label filter (using a nonexistent label)
	result, err := Ps("label=devcontainer.local_folder=/nonexistent/path")
	if err != nil {
		// Skip if Docker daemon is not running, etc.
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
			t.Skip("Docker daemon not running, skipping test")
		}
		t.Fatalf("Ps with label filter failed: %v", err)
	}

	// Expected to be an empty string as the label does not exist
	if result != "" {
		t.Logf("Unexpected result with nonexistent label: %s", result)
	}
}
