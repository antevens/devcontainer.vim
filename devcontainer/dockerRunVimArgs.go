package devcontainer

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/mikoto2000/devcontainer.vim/v3/docker"
)

const dockerDetachKeys = "ctrl-\\\\"

func buildDockerRunVimExecArgs(containerID string, shell string) []string {
	if shell == "" {
		return []string{
			"exec",
			"--detach-keys",
			dockerDetachKeys,
			"-it",
			containerID,
			"/VimRun.sh",
		}
	}

	return []string{
		"exec",
		"--detach-keys",
		dockerDetachKeys,
		"-it",
		containerID,
		shell,
	}
}

// Assemble the arguments for `docker exec` during `devcontainer.vim run`
//
// Args:
//   - containerID: Container ID
//   - vimFileName: Filename of vim transferred to the container
//   - useSystemVim: If true, use vim/nvim installed on the system
//
// Return:
//
//	Array of command line arguments used for `docker exec`
func dockerRunVimArgs(containerID string, vimFileName string, tmuxFileName string, sendToTCP string, containerArch string, useSystemVim bool, useSystemTmux bool, noTmux bool, shell string, configFilePath string) ([]string, error) {
	var templateSource string
	var err error
	if useSystemVim {
		templateSource = vimRunX8664System
	} else {
		if containerArch == "amd64" {
			if runtime.GOOS != "darwin" {
				templateSource = vimRunX8664AppImage
			} else {
				templateSource = vimRunX8664Static
			}
		} else {
			templateSource = vimRunAarch64
		}
	}

	tmuxCommand := "/" + tmuxFileName
	if useSystemTmux {
		tmuxCommand = tmuxFileName
	}
	vimRunScript, err := renderVimRunScript(templateSource, vimRunScriptParams{
		VimFileName: vimFileName,
		SendToTcp:   sendToTCP,
		UseTmux:     !noTmux,
		TmuxCommand: tmuxCommand,
	})
	if err != nil {
		return nil, err
	}

	// Output Vim launch script
	vimLaunchScript := filepath.Join(configFilePath, "VimRun.sh")
	os.RemoveAll(vimLaunchScript)
	err = os.WriteFile(vimLaunchScript, []byte(vimRunScript), 0766)
	if err != nil {
		return nil, err
	}

	docker.Cp("Vim launch script", vimLaunchScript, containerID, "/VimRun.sh")

	return buildDockerRunVimExecArgs(containerID, shell), nil
}
