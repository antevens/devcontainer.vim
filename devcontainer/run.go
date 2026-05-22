package devcontainer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mikoto2000/devcontainer.vim/v3/tools"
)

var devcontainerRunArgsPrefix = []string{"run", "-d", "--rm", "--add-host=host.docker.internal:host-gateway"}
var devcontainerRunArgsSuffix = []string{"sh", "-c", "trap \"exit 0\" TERM; sleep infinity & wait"}

type ContainerStartError struct {
	msg string
}

func (e *ContainerStartError) Error() string {
	return e.msg
}

type ChmodError struct {
	msg string
}

func (e *ChmodError) Error() string {
	return e.msg
}

// Set up a container in a single shot with `docker run`
func Run(
	args []string,
	noCdr bool,
	noPf bool,
	noTmux bool,
	cdrPath string,
	vimInstallDir string,
	nvim bool,
	shell string,
	configDirForDocker string,
	vimrc string,
	defaultRunargs []string) error {

	// Container setup
	containerID, vimFileName, tmuxFileName, sendToTCP, containerArch, useSystemVim, useSystemTmux, cdrPid, cdrConfigDir, err := setupContainer(
		args,
		noCdr,
		noPf,
		noTmux,
		cdrPath,
		vimInstallDir,
		nvim,
		configDirForDocker,
		vimrc,
		defaultRunargs)

	// Cleanup
	// Stop clipboard-data-receiver
	defer func() {
		err = tools.KillCdr(cdrPid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Container stop error: %s\n", err)
		}

		err = os.RemoveAll(cdrConfigDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cache remove error: %s\n", err)
		}
	}()

	// Stop container
	defer func() {
		// `docker stop <CONTAINER ID displayed on stdout during dockerrun>`
		fmt.Printf("Stop container(Async) %s.\n", containerID)
		err = exec.Command(containerCommand, "stop", containerID).Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Container stop error: %s\n", err)
		}
	}()

	// Connect to the container
	// `docker exec <CONTAINER ID displayed on stdout during dockerrun> /Vim-AppImage`

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	sendToTCPName := filepath.Base(sendToTCP)
	dockerRunVimArgs, err := dockerRunVimArgs(containerID, vimFileName, tmuxFileName, sendToTCPName, containerArch, useSystemVim, useSystemTmux, noTmux, shell, configDirForDocker)
	if err != nil {
		return err
	}
	fmt.Printf("Start vim: `%s \"%s\"`\n", containerCommand, strings.Join(dockerRunVimArgs, "\" \""))
	dockerExec := exec.CommandContext(ctx, containerCommand, dockerRunVimArgs...)
	dockerExec.Stdin = os.Stdin
	dockerExec.Stdout = os.Stdout
	dockerExec.Stderr = os.Stderr
	dockerExec.Cancel = func() error {
		fmt.Fprintf(os.Stderr, "Receive SIGINT.\n")
		return dockerExec.Process.Signal(os.Interrupt)
	}

	err = dockerExec.Run()
	if err != nil {
		return err
	}

	return nil
}

// Start the container and return the container ID
func startContainer(args []string, defaultRunargs []string) (string, error) {
	devcontainerRunArgs := devcontainerRunArgsPrefix
	// Use runargs if not on Windows
	if runtime.GOOS != "windows" {
		devcontainerRunArgs = append(devcontainerRunArgs, defaultRunargs...)
	}
	devcontainerRunArgs = append(devcontainerRunArgs, args...)
	devcontainerRunArgs = append(devcontainerRunArgs, devcontainerRunArgsSuffix...)
	fmt.Printf("run container: `%s \"%s\"`\n", containerCommand, strings.Join(devcontainerRunArgs, "\" \""))

	dockerRunCommand := exec.Command(containerCommand, devcontainerRunArgs...)
	containerIDRaw, err := dockerRunCommand.Output()
	containerID := string(containerIDRaw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Container start error.")
		fmt.Fprintln(os.Stderr, string(containerID))
		return "", &ContainerStartError{msg: "Container start error."}
	}

	containerID = strings.ReplaceAll(containerID, "\n", "")
	containerID = strings.ReplaceAll(containerID, "\r", "")
	fmt.Printf("Container started. id: %s\n", containerID)

	return containerID, nil
}

// Start clipboard-data-receiver
func startClipboardReceiver(cdrPath, configDirForDocker, containerID string) (int, int, string, error) {
	configDirForCdr := filepath.Join(configDirForDocker, containerID)
	err := os.MkdirAll(configDirForCdr, 0744)
	if err != nil {
		return 0, 0, configDirForCdr, err
	}
	pid, port, err := tools.RunCdr(cdrPath, configDirForCdr)
	if err != nil {
		return 0, 0, configDirForCdr, err
	}
	fmt.Printf("Started clipboard-data-receiver with pid: %d, port: %d\n", pid, port)
	return pid, port, configDirForCdr, nil
}

func setupContainer(
	args []string,
	noCdr bool,
	noPf bool,
	noTmux bool,
	cdrPath string,
	vimInstallDir string,
	nvim bool,
	configDirForDocker string,
	vimrc string,
	defaultRunargs []string) (string, string, string, string, string, bool, bool, int, string, error) {

	// 1. Start the container
	containerID, err := startContainer(args, defaultRunargs)
	if err != nil {
		return "", "", "", "", "", false, false, 0, "", err
	}

	// 2. Get container architecture
	containerArch, err := getContainerArch(containerID)
	if err != nil {
		return containerID, "", "", "", "", false, false, 0, "", err
	}

	// 3. Install port-forwarder
	if !noPf {
		err = installPortForwarder(containerID, vimInstallDir, containerArch)
		if err != nil {
			return containerID, "", "", "", containerArch, false, false, 0, "", err
		}
	}

	// 4. Start clipboard-data-receiver
	pid := 0
	port := 0
	configDirForCdr := ""
	if !noCdr {
		pid, port, configDirForCdr, err = startClipboardReceiver(cdrPath, configDirForDocker, containerID)
		if err != nil {
			return containerID, "", "", "", containerArch, false, false, pid, configDirForCdr, err
		}
	}

	// 5. Detect and install Vim
	vimFileName, useSystemVim, err := setupVim(containerID, vimInstallDir, nvim, containerArch)
	if err != nil {
		return containerID, vimFileName, "", "", containerArch, useSystemVim, false, pid, configDirForCdr, err
	}

	tmuxFileName := ""
	useSystemTmux := false
	if !noTmux {
		tmuxFileName, useSystemTmux, err = setupTmux(containerID, vimInstallDir, containerArch)
		if err != nil {
			return containerID, vimFileName, "", "", containerArch, useSystemVim, false, pid, configDirForCdr, err
		}
	}

	// 6. Transfer Vim files
	sendToTCP, err := transferVimFiles(containerID, "", configDirForDocker, vimrc, noCdr, port, vimFileName == "nvim", []map[string]interface{}{})
	if err != nil {
		return containerID, vimFileName, tmuxFileName, sendToTCP, containerArch, useSystemVim, useSystemTmux, pid, configDirForCdr, err
	}

	return containerID, vimFileName, tmuxFileName, sendToTCP, containerArch, useSystemVim, useSystemTmux, pid, configDirForCdr, nil
}
