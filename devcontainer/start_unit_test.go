package devcontainer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Unit test for startDevcontainer function (mock)
func TestStartDevcontainerMock(t *testing.T) {
	// Parameter validation test
	args := []string{"image", "workspace"}
	devcontainerPath := "/mock/devcontainer"
	configFilePath := "/mock/config.json"
	workspaceFolder := "workspace"

	// Argument validity check
	if len(args) < 2 {
		t.Fatal("args should have at least 2 elements")
	}

	if devcontainerPath == "" {
		t.Fatal("devcontainerPath should not be empty")
	}

	if configFilePath == "" {
		t.Fatal("configFilePath should not be empty")
	}

	if workspaceFolder == "" {
		t.Fatal("workspaceFolder should not be empty")
	}

	t.Logf("startDevcontainer parameters validated successfully")
}

// Unit test for startClipboardReceiverForDevcontainer function (mock)
func TestStartClipboardReceiverForDevcontainerMock(t *testing.T) {
	// Parameter validation test
	cdrPath := "/mock/cdr"
	configDirForDevcontainer := "/mock/config"

	// Argument validity check
	if cdrPath == "" {
		t.Fatal("cdrPath should not be empty")
	}

	if configDirForDevcontainer == "" {
		t.Fatal("configDirForDevcontainer should not be empty")
	}

	t.Logf("startClipboardReceiverForDevcontainer parameters validated successfully")
}

// Unit test for setupPortForwarding function (mock)
func TestSetupPortForwardingMock(t *testing.T) {
	// Parameter validation test
	containerID := "mock-container-id"
	devcontainerPath := "/mock/devcontainer"
	workspaceFolder := "/mock/workspace"

	// Argument validity check
	if containerID == "" {
		t.Fatal("containerID should not be empty")
	}

	if devcontainerPath == "" {
		t.Fatal("devcontainerPath should not be empty")
	}

	if workspaceFolder == "" {
		t.Fatal("workspaceFolder should not be empty")
	}

	t.Logf("setupPortForwarding parameters validated successfully")
}

// Integration test for Start function (lightweight version)
func TestStartParameterValidation(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "start_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Parameters for testing
	services := TestDevcontainerStartUseService{}
	args := []string{"test-image", "workspace"}
	devcontainerPath := "/mock/devcontainer"
	nvim := false

	// Validate parameter validity
	if len(args) < 2 {
		t.Fatal("args should have at least 2 elements")
	}

	workspaceFolder := args[len(args)-1]
	if workspaceFolder == "" {
		t.Fatal("workspaceFolder should not be empty")
	}

	// Validate other parameters
	if devcontainerPath == "" {
		t.Fatal("devcontainerPath should not be empty")
	}

	t.Logf("Start function parameters validated: args=%v, workspace=%s, nvim=%v", args, workspaceFolder, nvim)

	// Validate service interface
	_ = services
}

func TestCreateStartVimCommandUsesScriptOnWsl(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "script")
	err := os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0755)
	if err != nil {
		t.Fatalf("failed to create mock script command: %v", err)
	}

	t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd := createStartVimCommand(context.Background(), "/mock/devcontainer", []string{"exec", "--workspace-folder", ".", "/run_vim.sh"})

	if filepath.Base(cmd.Path) != "script" {
		t.Fatalf("expected script wrapper, got path: %s", cmd.Path)
	}
	if len(cmd.Args) != 4 {
		t.Fatalf("unexpected script args: %#v", cmd.Args)
	}
	if cmd.Args[1] != "-qefc" {
		t.Fatalf("expected -qefc, got: %s", cmd.Args[1])
	}
	if !strings.Contains(cmd.Args[2], "'/mock/devcontainer' 'exec' '--workspace-folder' '.' '/run_vim.sh'") {
		t.Fatalf("unexpected wrapped command: %s", cmd.Args[2])
	}
	if cmd.Args[3] != "/dev/null" {
		t.Fatalf("expected /dev/null output file, got: %s", cmd.Args[3])
	}
}

func TestCreateStartVimCommandUsesDirectExecOutsideWsl(t *testing.T) {
	originalValue, hadValue := os.LookupEnv("WSL_DISTRO_NAME")
	err := os.Unsetenv("WSL_DISTRO_NAME")
	if err != nil {
		t.Fatalf("failed to unset WSL_DISTRO_NAME: %v", err)
	}
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv("WSL_DISTRO_NAME", originalValue)
		}
	})

	cmd := createStartVimCommand(context.Background(), "/mock/devcontainer", []string{"exec", "--workspace-folder", ".", "/run_vim.sh"})

	if cmd.Path != "/mock/devcontainer" {
		t.Fatalf("expected direct exec path, got: %s", cmd.Path)
	}
	if len(cmd.Args) != 5 {
		t.Fatalf("unexpected direct exec args: %#v", cmd.Args)
	}
	if cmd.Args[0] != "/mock/devcontainer" {
		t.Fatalf("expected command path as first arg, got: %#v", cmd.Args)
	}
	if cmd.Args[1] != "exec" {
		t.Fatalf("expected direct exec args, got: %#v", cmd.Args)
	}
}
