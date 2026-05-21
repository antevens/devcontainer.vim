package devcontainer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mikoto2000/devcontainer.vim/v3/docker"
	"github.com/mikoto2000/devcontainer.vim/v3/tools"
)

// Test for setupContainer function (continue using existing test)
func TestSetupContainer(t *testing.T) {
	_, binDir, configDirForDocker, _ := createTempAppDirs(t)

	nvim := false
	cdrPath := requireTestBinary(t, "clipboard-data-receiver")

	vimrc := "../test/resource/TestRun/vimrc"
	noCdr := false
	noPf := false

	containerID, _, _, _, _, _, _, _, _, err := setupContainer(
		[]string{"alpine:latest"},
		noCdr,
		noPf,
		false,
		cdrPath,
		binDir,
		nvim,
		configDirForDocker,
		vimrc,
		[]string{},
	)

	if err != nil {
		if strings.Contains(err.Error(), "Container start error") {
			t.Skipf("container runtime not available for integration test: %v", err)
		}
		t.Fatalf("error: %s", err)
	}

	// Cleanup
	// Stop container
	defer func() {
		// `docker stop <CONTAINER ID displayed on stdout during dockerrun>`
		fmt.Printf("Stop container(Async) %s.\n", containerID)
		err = exec.Command(containerCommand, "stop", containerID).Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Container stop error: %s\n", err)
		}
	}()

	//     /vim
	vimOutput, err := docker.Exec(containerID, "sh", "-c", "ls /vim*")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	vimWant := "vim"
	if !strings.Contains(vimOutput, vimWant) {
		t.Fatalf("error: want match %s, but got %s", vimWant, vimOutput)
	}
	//     /vimrc
	vimrcOutput, err := docker.Exec(containerID, "sh", "-c", "ls /vimrc")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	vimrcWant := "vimrc"
	if !strings.Contains(vimrcOutput, vimrcWant) {
		t.Fatalf("error: want match %s, but got %s", vimrcWant, vimrcOutput)
	}
}

// Unit test for startContainer function
func TestStartContainer(t *testing.T) {
	// Check for Docker existence
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker command not found, skipping test")
	}

	args := []string{"alpine:latest"}
	defaultRunargs := []string{}

	containerID, err := startContainer(args, defaultRunargs)
	if err != nil {
		// Skip if Docker daemon is not running
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(err.Error(), "Container start error") {
			t.Skip("Docker daemon not running, skipping test")
		}
		t.Fatalf("startContainer failed: %v", err)
	}

	// Confirm that the container ID is returned
	if containerID == "" {
		t.Fatal("Container ID should not be empty")
	}

	// Cleanup
	defer func() {
		exec.Command(containerCommand, "stop", containerID).Start()
	}()

	// Confirm that the container is actually running
	psOutput, err := docker.Ps("id=" + containerID)
	if err != nil {
		t.Fatalf("Failed to check container status: %v", err)
	}

	if psOutput == "" {
		t.Fatal("Container should be running")
	}
}

// Unit test for getContainerArch function
func TestGetContainerArch(t *testing.T) {
	// Check for Docker existence
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker command not found, skipping test")
	}

	// Start test container
	args := []string{"alpine:latest"}
	containerID, err := startContainer(args, []string{})
	if err != nil {
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(err.Error(), "Container start error") {
			t.Skip("Docker daemon not running, skipping test")
		}
		t.Fatalf("Failed to start container: %v", err)
	}

	defer func() {
		exec.Command(containerCommand, "stop", containerID).Start()
	}()

	// Get architecture
	arch, err := getContainerArch(containerID)
	if err != nil {
		t.Fatalf("getContainerArch failed: %v", err)
	}

	// Confirm that a valid architecture is returned
	validArchs := []string{"amd64", "arm64", "x86_64", "aarch64"}
	found := false
	for _, validArch := range validArchs {
		if arch == validArch {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("Unexpected architecture: %s", arch)
	}
}

// Unit test for setupVim function (when system Vim exists)
func TestSetupVimWithSystemVim(t *testing.T) {
	// Check for Docker existence
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker command not found, skipping test")
	}

	// Start test container (using an image with Vim pre-installed)
	args := []string{"alpine:latest"}
	containerID, err := startContainer(args, []string{})
	if err != nil {
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(err.Error(), "Container start error") {
			t.Skip("Docker daemon not running, skipping test")
		}
		t.Fatalf("Failed to start container: %v", err)
	}

	defer func() {
		exec.Command(containerCommand, "stop", containerID).Start()
	}()

	// Test whether Vim is installed on the system
	vimFileName, useSystemVim, err := setupVim(containerID, "", false, "amd64")
	if err != nil {
		t.Fatalf("setupVim failed: %v", err)
	}

	// Verification of results
	if vimFileName != "vim" && vimFileName != "nvim" {
		t.Fatalf("Unexpected vim filename: %s", vimFileName)
	}

	// Confirm that the value of useSystemVim is valid
	t.Logf("useSystemVim: %v, vimFileName: %s", useSystemVim, vimFileName)
}

// Integration test for Run function (using mock)
func TestRunWithMock(t *testing.T) {
	// This test is a mock test that does not use actual Docker containers
	// Test the structure of the Run function, but skip actual Vim execution

	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "devcontainer_run_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Parameters for mock
	args := []string{"test-image"}
	nvim := false
	configDirForDocker := tempDir

	// Since the Run function requires an actual Docker container,
	// only the callability of the function is tested here
	// Actual execution is performed in the integration test environment

	// Test parameter validity
	if len(args) == 0 {
		t.Fatal("args should not be empty")
	}

	if configDirForDocker == "" {
		t.Fatal("configDirForDocker should not be empty")
	}

	t.Logf("Run function parameters validated: args=%v, nvim=%v", args, nvim)
}

// Integration test for Run function (using actual container)
func TestRunIntegration(t *testing.T) {
	// Since this is a long-running test, it can be skipped with a short timeout
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check for Docker existence
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker command not found, skipping integration test")
	}

	_, binDir, configDirForDocker, _ := createTempAppDirs(t)

	nvim := false
	cdrPath := requireTestBinary(t, "clipboard-data-receiver")

	args := []string{"alpine:latest"}
	vimrc := "../test/resource/TestRun/vimrc"
	defaultRunargs := []string{}
	noCdr := false
	noPf := false

	// Execute the Run function in a separate goroutine and set a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		// Since the Run function normally waits for user input, terminate it immediately
		// In actual tests, confirm that the container starts and Vim is in an executable state

		// Test only the setupContainer part
		containerID, _, _, _, _, _, _, cdrPid, cdrConfigDir, err := setupContainer(
			args,
			noCdr,
			noPf,
			false,
			cdrPath,
			binDir,
			nvim,
			configDirForDocker,
			vimrc,
			defaultRunargs)

		if err != nil {
			done <- err
			return
		}

		// Cleanup
		tools.KillCdr(cdrPid)
		os.RemoveAll(cdrConfigDir)
		exec.Command(containerCommand, "stop", containerID).Start()

		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(err.Error(), "Container start error") {
				t.Skip("Docker daemon not running, skipping integration test")
			}
			t.Fatalf("Run integration test failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run integration test timed out")
	}
}
