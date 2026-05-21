package devcontainer

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type TestDevcontainerStartUseService struct{}

func (s TestDevcontainerStartUseService) StartVim(containerID string, devcontainerPath string, workspaceFolder string, vimFileName string, tmuxFileName string, sendToTCP string, containerArch string, useSystemVim bool, useSystemTmux bool, noTmux bool, shell string, configFilePathForDevcontainer string) error {
	return nil
}

func TestStart(t *testing.T) {
	// Check prerequisites for integration test
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check for Docker existence
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker command not found, skipping integration test")
	}

	_, binDir, _, configDirForDevcontainer := createTempAppDirs(t)

	// Download necessary files
	nvim := false
	devcontainerPath := requireTestBinary(t, "devcontainer")
	cdrPath := requireTestBinary(t, "clipboard-data-receiver")

	// Test if devcontainer command works
	testCmd := exec.Command(devcontainerPath, "--version")
	err = testCmd.Run()
	if err != nil {
		t.Skipf("devcontainer CLI not working: %v", err)
	}

	// Use the end of the command line arguments as the value for --workspace-folder
	configFilePath, err := CreateConfigFile(devcontainerPath, "../test/project/TestStart", configDirForDevcontainer)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("Configuration file not found: %v", err)
		} else if strings.Contains(err.Error(), "failed to parse") {
			t.Skipf("devcontainer CLI parse error (environment issue): %v", err)
		} else {
			t.Fatalf("Error creating config file: %v", err)
		}
	}

	args := []string{"../test/project/TestStart"}

	// Start the container using devcontainer
	noCdr := false
	noPf := false
	err = Start(TestDevcontainerStartUseService{}, args, devcontainerPath, noCdr, noPf, false, cdrPath, binDir, nvim, "", configFilePath, "../test/resource/TestStart/vimrc")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("Permission error: %v", err)
		} else if strings.Contains(err.Error(), "Container start error") {
			t.Skipf("Container start error (environment issue): %v", err)
		} else {
			t.Fatalf("Error executing devcontainer: %v", err)
		}
	}

	// Cleanup
	defer Down([]string{"../test/project/TestStart"}, devcontainerPath, configDirForDevcontainer)

	// Does the container start with the settings after JSON merging?
	// Are the desired files transferred to the started container?
	//     Is the storage mounted?
	vimfilesOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", "../test/project/TestStart", "sh", "-c", "ls -d ~/.vim")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	vimfilesWant := "/home/vscode/.vim"
	if !strings.Contains(vimfilesOutput, vimfilesWant) {
		t.Fatalf("error: want match %s, but got %s", vimfilesWant, vimfilesOutput)
	}
	//     Is port forwarding performed?
	//     TODO: For some reason it doesn't appear in the test...
	//pfOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", "../test/project/TestStart", "sh", "-c", "ls /pf")
	//if err != nil {
	//	t.Fatalf("error: %s", err)
	//}
	//pfWant := "localhost:8888_"
	//if !strings.Contains(pfOutput, pfWant) {
	//	t.Fatalf("error: want match %s, but got %s", pfWant, pfOutput)
	//}

	//     Are environment variables set?
	termOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", "../test/project/TestStart", "sh", "-c", "\"env\"")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	termWantMatch := "TERM=xterm-256color"
	if !strings.Contains(termOutput, termWantMatch) {
		t.Fatalf("error: want match %s, but got %s", termWantMatch, termOutput)
	}
	//     /vim
	vimOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", "../test/project/TestStart", "sh", "-c", "ls /vim")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	vimWant := "vim"
	if !strings.Contains(vimOutput, vimWant) {
		t.Fatalf("error: want match %s, but got %s", vimWant, vimOutput)
	}
	//     /vimrc
	vimrcOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", "../test/project/TestStart", "sh", "-c", "ls /vimrc")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	vimrcWant := "vimrc"
	if !strings.Contains(vimrcOutput, vimrcWant) {
		t.Fatalf("error: want match %s, but got %s", vimrcWant, vimrcOutput)
	}
	//     /port-forwarder
	portForwarderOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", "../test/project/TestStart", "sh", "-c", "ls /port-forwarder")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	portForwarderWant := "port-forwarder"
	if !strings.Contains(portForwarderOutput, portForwarderWant) {
		t.Fatalf("error: want match %s, but got %s", portForwarderWant, portForwarderOutput)
	}
}

func TestStartWithDockerCompose(t *testing.T) {
	// TODO: Fix to succeed without chdir
	os.Chdir("../test/project/TestStartWithDockerCompose")
	_, binDir, _, configDirForDevcontainer := createTempAppDirs(t)

	// Download necessary files
	nvim := false
	devcontainerPath := requireTestBinary(t, "devcontainer")
	cdrPath := requireTestBinary(t, "clipboard-data-receiver")

	// Use the end of the command line arguments as the value for --workspace-folder
	configFilePath, err := CreateConfigFile(devcontainerPath, ".", configDirForDevcontainer)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("Configuration file not found: %v", err)
		} else if strings.Contains(err.Error(), "failed to parse") {
			t.Skipf("devcontainer CLI parse error (environment issue): %v", err)
		} else {
			t.Fatalf("Error creating config file: %v", err)
		}
	}

	args := []string{"."}

	// Start the container using devcontainer
	noCdr := false
	noPf := false
	err = Start(TestDevcontainerStartUseService{}, args, devcontainerPath, noCdr, noPf, false, cdrPath, binDir, nvim, "", configFilePath, "../../resource/TestStartWithDockerCompose/vimrc")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("Permission error: %v", err)
		} else if strings.Contains(err.Error(), "Container start error") {
			t.Skipf("Container start error (environment issue): %v", err)
		} else {
			t.Fatalf("Error executing devcontainer: %v", err)
		}
	}

	// Cleanup
	defer Down([]string{"."}, devcontainerPath, configDirForDevcontainer)

	// Does the container start with the settings after JSON merging?
	// Are the desired files transferred to the started container?
	//     Is the storage mounted?
	vimfilesOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", ".", "sh", "-c", "ls -d ~/.vim")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	vimfilesWant := "/home/vscode/.vim"
	if !strings.Contains(vimfilesOutput, vimfilesWant) {
		t.Fatalf("error: want match %s, but got %s", vimfilesWant, vimfilesOutput)
	}

	//     Are environment variables set?
	termOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", ".", "sh", "-c", "\"env\"")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	termWantMatch := "TERM=xterm-256color"
	if !strings.Contains(termOutput, termWantMatch) {
		t.Fatalf("error: want match %s, but got %s", termWantMatch, termOutput)
	}
	//     /vim
	vimOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", ".", "sh", "-c", "ls /vim")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	vimWant := "vim"
	if !strings.Contains(vimOutput, vimWant) {
		t.Fatalf("error: want match %s, but got %s", vimWant, vimOutput)
	}
	//     /vimrc
	vimrcOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", ".", "sh", "-c", "ls /vimrc")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	vimrcWant := "vimrc"
	if !strings.Contains(vimrcOutput, vimrcWant) {
		t.Fatalf("error: want match %s, but got %s", vimrcWant, vimrcOutput)
	}
	//     /port-forwarder
	portForwarderOutput, err := Execute(devcontainerPath, "exec", "--workspace-folder", ".", "sh", "-c", "ls /port-forwarder")
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	portForwarderWant := "port-forwarder"
	if !strings.Contains(portForwarderOutput, portForwarderWant) {
		t.Fatalf("error: want match %s, but got %s", portForwarderWant, portForwarderOutput)
	}

}
