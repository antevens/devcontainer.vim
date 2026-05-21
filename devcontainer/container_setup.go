package devcontainer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mikoto2000/devcontainer.vim/v3/docker"
	"github.com/mikoto2000/devcontainer.vim/v3/tools"
	"github.com/mikoto2000/devcontainer.vim/v3/util"
)

// Get container architecture
func getContainerArch(containerID string) (string, error) {
	containerArch, err := docker.Exec(containerID, "uname", "-m")
	if err != nil {
		return "", err
	}
	containerArch = strings.TrimSpace(containerArch)
	containerArch, err = util.NormalizeContainerArch(containerArch)
	if err != nil {
		return "", err
	}
	fmt.Printf("Container Arch: '%s'.\n", containerArch)
	return containerArch, nil
}

// Install port-forwarder in the container
func installPortForwarder(containerID, vimInstallDir, containerArch string) error {
	portForwarderContainerPath, err := tools.PortForwarderContainer(tools.DefaultInstallerUseServices{}).Install(vimInstallDir, containerArch, false)
	if err != nil {
		return err
	}
	err = docker.Cp("port-forwarder-container", portForwarderContainerPath, containerID, "/port-forwarder")
	if err != nil {
		return err
	}
	return nil
}

// Detect and install tmux
func setupTmux(containerID, vimInstallDir string, containerArch string) (string, bool, error) {
	useSystemTmux := false
	fmt.Printf("Check system installed tmux ... ")
	out, _ := docker.Exec(containerID, "which", "tmux")
	if out != "" {
		fmt.Printf("found.\n")
		useSystemTmux = true
	} else {
		fmt.Printf("not found.\n")
	}
	fmt.Printf("docker exec output: \"%s\".\n", strings.TrimSpace(out))

	if useSystemTmux {
		return "tmux", true, nil
	}

	tmuxFilePath, err := tools.InstallTmux(vimInstallDir, containerArch)
	if err != nil {
		return "", false, err
	}

	err = docker.Cp("tmux", tmuxFilePath, containerID, "/tmux")
	if err != nil {
		return "", false, err
	}

	dockerChownArgs := []string{"exec", "--user", "root", containerID, "sh", "-c", "chmod +x /tmux"}
	fmt.Printf("Chown tmux: `%s \"%s\"` ...", containerCommand, strings.Join(dockerChownArgs, "\" \""))
	chmodResult, err := exec.Command(containerCommand, dockerChownArgs...).CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, "chmod error.")
		fmt.Fprintln(os.Stderr, string(chmodResult))
		return "", false, &ChmodError{msg: "chmod error."}
	}
	fmt.Printf(" done.\n")

	return "tmux", false, nil
}

// Detect and install Vim
func setupVim(containerID, vimInstallDir string, nvim bool, containerArch string) (string, bool, error) {
	vimFileName := "vim"
	if nvim {
		vimFileName = "nvim"
	}

	useSystemVim := false
	fmt.Printf("Check system installed %s ... ", vimFileName)
	out, _ := docker.Exec(containerID, "which", vimFileName)
	if out != "" {
		fmt.Printf("found.\n")
		useSystemVim = true

		if nvim {
			vimFileName = "nvim"
		}
	} else {
		fmt.Printf("not found.\n")

		if runtime.GOARCH == "arm64" {
			// Fallback to vim because static-linked nvim cannot be built for arm
			vimFileName = "vim"
			nvim = false
		} else if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" && nvim {
			// Fallback to vim because AppImage does not work on M1 Mac with amd64 container for some reason
			vimFileName = "vim"
			nvim = false
		}
	}
	fmt.Printf("docker exec output: \"%s\".\n", strings.TrimSpace(out))

	if !useSystemVim {
		// Transfer Vim/Neovim to the container and add execution permission
		vimFilePath, err := tools.InstallVim(vimInstallDir, nvim, containerArch)
		if err != nil {
			return "", false, err
		}

		// Different processing for start.go and run.go: start.go requires special path analysis
		actualVimFileName := vimFileName
		if strings.Contains(vimFilePath, "_") {
			// Case where the path comes in the format vim_<ARCH>, nvim_<ARCH> (start.go)
			actualVimFileName = strings.Split(filepath.Base(vimFilePath), "_")[0]
		}

		err = docker.Cp("vim", vimFilePath, containerID, "/"+actualVimFileName)
		if err != nil {
			return actualVimFileName, useSystemVim, err
		}

		// `docker exec <CONTAINER ID displayed on stdout during dockerrun> chmod +x /Vim-AppImage`
		dockerChownArgs := []string{"exec", "--user", "root", containerID, "sh", "-c", "chmod +x /" + actualVimFileName}
		fmt.Printf("Chown AppImage: `%s \"%s\"` ...", containerCommand, strings.Join(dockerChownArgs, "\" \""))
		chmodResult, err := exec.Command(containerCommand, dockerChownArgs...).CombinedOutput()
		if err != nil {
			fmt.Fprintln(os.Stderr, "chmod error.")
			fmt.Fprintln(os.Stderr, string(chmodResult))
			return actualVimFileName, useSystemVim, &ChmodError{msg: "chmod error."}
		}
		fmt.Printf(" done.\n")

		return actualVimFileName, useSystemVim, nil
	}

	return vimFileName, useSystemVim, nil
}

// Transfer Vim files (SendToTcp.vim and vimrc) to the container
func transferVimFiles(containerID, configDir, vimrc string, noCdr bool, port int, isNvim bool) (string, error) {
	// Transfer Vim-related files (SendToTcp.vim and additional vimrc)
	sendToTCP, err := tools.CreateSendToTCP(configDir, port, noCdr, isNvim)
	if err != nil {
		return "", err
	}

	// Transfer SendToTcp.vim to the container
	err = docker.Cp("SendToTcp.vim", sendToTCP, containerID, "/")
	if err != nil {
		return sendToTCP, err
	}

	// Transfer vimrc to the container
	err = docker.Cp("vimrc", vimrc, containerID, "/")
	if err != nil {
		return sendToTCP, err
	}

	return sendToTCP, nil
}
