package devcontainer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/mikoto2000/devcontainer.vim/v3/docker"
	"github.com/mikoto2000/devcontainer.vim/v3/tools"
	"github.com/mikoto2000/devcontainer.vim/v3/util"
)

const portForwarderMarkerDir = "~/.config/devcontainer.vim/pf"

type DevcontainerStartUseService interface {
	StartVim(containerID string, devcontainerPath string, workspaceFolder string, vimFileName string, tmuxFileName string, sendToTCP string, containerArch string, useSystemVim bool, useSystemTmux bool, noTmux bool, shell string, configFilePathForDevcontainer string) error
}

type DefaultDevcontainerStartUseService struct{}

func (s DefaultDevcontainerStartUseService) StartVim(containerID string, devcontainerPath string, workspaceFolder string, vimFileName string, tmuxFileName string, sendToTCP string, containerArch string, useSystemVim bool, useSystemTmux bool, noTmux bool, shell string, configDirForDevcontainer string) error {
	return startVim(containerID, devcontainerPath, workspaceFolder, vimFileName, tmuxFileName, sendToTCP, containerArch, useSystemVim, useSystemTmux, noTmux, shell, configDirForDevcontainer)
}

var devcontainreArgsPrefix = []string{"up"}

// Start the container with `devcontainer up` and return the container ID
func startDevcontainer(devcontainerPath string, args []string, configFilePath string, workspaceFolder string) (string, error) {
	// Pass all arguments except the last one as they are to `devcontainer up`
	userArgs := args[0 : len(args)-1]
	userArgs = append(userArgs, "--override-config", configFilePath, "--workspace-folder", workspaceFolder)
	devcontainerArgs := append(devcontainreArgsPrefix, userArgs...)
	fmt.Printf("run container: `%s \"%s\"`\n", devcontainerPath, strings.Join(devcontainerArgs, "\" \""))

	dockerRunCommand := exec.Command(devcontainerPath, devcontainerArgs...)
	dockerRunCommand.Stderr = os.Stderr

	stdout, err := dockerRunCommand.Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Container start error.")
		return "", err
	}

	upCommandResult, err := UnmarshalUpCommandResult(stdout)
	if err != nil {
		return "", err
	}

	containerID := upCommandResult.ContainerID
	fmt.Printf("finished devcontainer up: %s\n", upCommandResult)

	return containerID, nil
}

// Start clipboard-data-receiver for devcontainer
func startClipboardReceiverForDevcontainer(cdrPath, configDirForDevcontainer string) (int, int, error) {
	pid, port, err := tools.RunCdr(cdrPath, configDirForDevcontainer)
	if err != nil {
		return 0, 0, err
	}
	fmt.Printf("Started clipboard-data-receiver with pid: %d, port: %d\n", pid, port)
	return pid, port, nil
}

func listRunningPortForwarders(containerID string) ([]string, error) {
	psOut, err := docker.Exec(containerID, "sh", "-c", "grep --files-with-matches port-forwarder /proc/*/comm || true")
	if err != nil {
		return nil, err
	}

	portForwarders := strings.Split(strings.TrimSpace(psOut), "\n")
	portForwarders = util.RemoveEmptyString(portForwarders)

	fmt.Printf("Running port-forwarders: %s\n", portForwarders)
	return portForwarders, nil
}

func listPortForwarderMarkers(containerID string) ([]string, error) {
	lspfOut, err := docker.Exec(containerID, "sh", "-c", "ls --zero "+portForwarderMarkerDir+" 2>/dev/null || true")
	if err != nil {
		return nil, err
	}

	forwardConfigs := strings.Split(lspfOut, "\x00")
	forwardConfigs = util.RemoveEmptyString(forwardConfigs)
	return forwardConfigs, nil
}

func startPortForwarders(ctx context.Context, containerID, containerIp, devcontainerPath, workspaceFolder string) error {
	fmt.Println("Start port-forwarder in container.")

	// Parse forwardPorts
	configurationString, err := ReadConfiguration(devcontainerPath, "--workspace-folder", workspaceFolder)
	if err != nil {
		return err
	}
	forwardConfigs, err := GetForwardPorts(configurationString)
	if err != nil {
		return err
	}

	// Start port-forwarder for each parsed forwardPort
	for _, fc := range forwardConfigs {

		// Start port-forwarder on the container side
		fmt.Printf("%s %s %s %s %s %s %s.\n", devcontainerPath, "exec", "--workspace-folder", ".", "sh", "-c", "/port-forwarder -l 0.0.0.0:0 -f "+fc.Host+":"+fc.Port)
		dockerExecPortForwarder := exec.CommandContext(ctx, devcontainerPath, "exec", "--workspace-folder", ".", "sh", "-c", "/port-forwarder -l 0.0.0.0:0 -f "+fc.Host+":"+fc.Port)
		portOut, err := dockerExecPortForwarder.StdoutPipe()
		if err != nil {
			return err
		}

		dockerExecPortForwarder.Cancel = func() error {
			fmt.Fprintf(os.Stderr, "Receive SIGINT.\n")
			return dockerExecPortForwarder.Process.Signal(os.Interrupt)
		}

		err = dockerExecPortForwarder.Start()
		if err != nil {
			return err
		}

		go func(host string, containerPort string) {
			reader := bufio.NewReader(portOut)
			for {
				port, err := reader.ReadString('\n')
				if err != nil {
					if err != io.EOF {
						fmt.Println("Error reading from stdout:", err)
					}
					break
				}
				port = strings.TrimSpace(port)
				fmt.Printf("port-forwarder started: %s:%s %s\n", containerIp, port, host+":"+containerPort)

				// Place the content of forwardPorts in `~/.config/devcontainer.vim/pf` directory in the format "<destination>_<listen address & port>"
				_, err = docker.Exec(containerID, "sh", "-c", "mkdir -p "+portForwarderMarkerDir+" && touch "+portForwarderMarkerDir+"/"+host+":"+containerPort+"_"+containerIp+":"+port)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating port-forwarder marker file: %v\n", err)
					continue
				}

				util.StartForwarding("0.0.0.0:"+containerPort, containerIp+":"+port)
			}
		}(fc.Host, fc.Port)
	}

	return nil
}

func restorePortForwarders(containerIp string, forwardConfigs []string) {
	for _, forwardConfig := range forwardConfigs {
		containerSrcPort, containerDestPort, err := parsePortForwarderMarker(forwardConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skip invalid port-forwarder marker: %s: %v\n", forwardConfig, err)
			continue
		}

		go func() {
			fmt.Printf("listen: %s, forward: %s.\n", "0.0.0.0:"+containerSrcPort, containerIp+":"+containerDestPort)
			util.StartForwarding("0.0.0.0:"+containerSrcPort, containerIp+":"+containerDestPort)
		}()
	}
}

func parsePortForwarderMarker(forwardConfig string) (string, string, error) {
	splitedForwardConfig := strings.Split(forwardConfig, "_")
	if len(splitedForwardConfig) != 2 {
		return "", "", errors.New("marker must contain source and destination")
	}

	containerSrc := splitedForwardConfig[0]
	scs := strings.Split(containerSrc, ":")
	if len(scs) != 2 {
		return "", "", errors.New("source marker must contain host and port")
	}

	containerDest := splitedForwardConfig[1]
	scd := strings.Split(containerDest, ":")
	if len(scd) != 2 {
		return "", "", errors.New("destination marker must contain host and port")
	}

	return scs[1], scd[1], nil
}

// Configure port forwarding
func setupPortForwarding(ctx context.Context, containerID, devcontainerPath, workspaceFolder string) error {
	// Get container IP address
	containerIp, err := docker.Exec(containerID, "sh", "-c", "hostname -i")
	if err != nil {
		return errors.New("failed to execute hostname on the container. The hostname command must be installed in the container")
	}
	containerIp = strings.TrimSpace(containerIp)

	portForwarders, err := listRunningPortForwarders(containerID)
	if err != nil {
		return err
	}
	forwardConfigs, err := listPortForwarderMarkers(containerID)
	if err != nil {
		return err
	}

	if len(portForwarders) == 0 {
		return startPortForwarders(ctx, containerID, containerIp, devcontainerPath, workspaceFolder)
	}

	if len(forwardConfigs) == 0 {
		fmt.Fprintf(os.Stderr, "port-forwarder process exists but marker files are missing. Restart port-forwarder setup.\n")
		return startPortForwarders(ctx, containerID, containerIp, devcontainerPath, workspaceFolder)
	}

	fmt.Println("port-forwarder already running.")
	restorePortForwarders(containerIp, forwardConfigs)

	return nil
}

// Start the container with devcontainer, transfer Vim, and execute.
// For historical reasons, configDirForDevcontainer is extracted from configFilePath
func Start(
	services DevcontainerStartUseService,
	args []string,
	devcontainerPath string,
	noCdr bool,
	noPf bool,
	noTmux bool,
	cdrPath string,
	vimInstallDir string,
	nvim bool,
	shell string,
	configFilePath string,
	vimrc string,
	dereferencedMounts []map[string]interface{}) error {

	// Use the end of the command line arguments as the value for --workspace-folder
	workspaceFolder := args[len(args)-1]

	// 1. Start the container with `devcontainer up`
	containerID, err := startDevcontainer(devcontainerPath, args, configFilePath, workspaceFolder)
	if err != nil {
		return err
	}

	// 2. Get container architecture
	containerArch, err := getContainerArch(containerID)
	if err != nil {
		return err
	}

	// 3. Install port-forwarder
	err = installPortForwarder(containerID, vimInstallDir, containerArch)
	if err != nil {
		return err
	}

	// 4. Start clipboard-data-receiver
	port := 0
	configDirForDevcontainer := filepath.Dir(configFilePath)
	if !noCdr {
		_, port, err = startClipboardReceiverForDevcontainer(cdrPath, configDirForDevcontainer)
		if err != nil {
			return err
		}
	}

	// 5. Configure port forwarding
	var pfCancel context.CancelFunc
	if !noPf {
		var pfCtx context.Context
		pfCtx, pfCancel = context.WithCancel(context.Background())
		err = setupPortForwarding(pfCtx, containerID, devcontainerPath, workspaceFolder)
		if err != nil {
			return err
		}
	}

	// 6. Detect and install Vim
	vimFileName, useSystemVim, err := setupVim(containerID, vimInstallDir, nvim, containerArch)
	if err != nil {
		return err
	}

	tmuxFileName := ""
	useSystemTmux := false
	if !noTmux {
		tmuxFileName, useSystemTmux, err = setupTmux(containerID, vimInstallDir, containerArch)
		if err != nil {
			return err
		}
	}

	// 7. Transfer Vim files and Dereferenced mounts
	containerHomeRaw, _ := Execute(devcontainerPath, "exec", "--workspace-folder", workspaceFolder, "sh", "-c", "echo ${HOME}")
	containerHome := strings.TrimSpace(containerHomeRaw)

	sendToTCP, err := transferVimFiles(containerID, containerHome, configDirForDevcontainer, vimrc, noCdr, port, vimFileName == "nvim", dereferencedMounts)
	if err != nil {
		return err
	}

	// 8. Connect to the container
	err = services.StartVim(containerID, devcontainerPath, workspaceFolder, vimFileName, tmuxFileName, sendToTCP, containerArch, useSystemVim, useSystemTmux, noTmux, shell, configDirForDevcontainer)
	if pfCancel != nil {
		pfCancel()
	}
	if err != nil {
		return err
	}

	// Container stop is performed separately by the down command
	return nil
}

// Connect to the container
// `docker exec <CONTAINER ID displayed on stdout during dockerrun> /Vim-AppImage`
func startVim(containerID string, devcontainerPath string, workspaceFolder string, vimFileName string, tmuxFileName string, sendToTCP string, containerArch string, useSystemVim bool, useSystemTmux bool, noTmux bool, shell string, configFilePathForDevcontainer string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	sendToTCPName := filepath.Base(sendToTCP)
	devcontainerStartVimArgs, err := devcontainerStartVimArgs(containerID, workspaceFolder, vimFileName, tmuxFileName, sendToTCPName, containerArch, useSystemVim, useSystemTmux, noTmux, shell, configFilePathForDevcontainer)
	if err != nil {
		return err
	}
	fmt.Printf("Start vim: `%s \"%s\"`\n", devcontainerPath, strings.Join(devcontainerStartVimArgs, "\" \""))
	dockerExec := createStartVimCommand(ctx, devcontainerPath, devcontainerStartVimArgs)
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
	} else {
		return nil
	}
}

func createStartVimCommand(ctx context.Context, devcontainerPath string, devcontainerStartVimArgs []string) *exec.Cmd {
	if util.IsWsl() && util.IsExistsCommand("script") {
		scriptArgs := []string{"-qefc", shellQuote(devcontainerPath)}
		for _, arg := range devcontainerStartVimArgs {
			scriptArgs[1] += " " + shellQuote(arg)
		}
		scriptArgs = append(scriptArgs, "/dev/null")
		return exec.CommandContext(ctx, "script", scriptArgs...)
	}

	return exec.CommandContext(ctx, devcontainerPath, devcontainerStartVimArgs...)
}

func shellQuote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}
