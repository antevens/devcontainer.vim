package devcontainer

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/mikoto2000/devcontainer.vim/v3/docker"
)

func buildDevcontainerStartVimExecArgs(containerID string, workspaceFolder string, shell string) []string {
	args := []string{
		"exec",
		"--container-id",
		containerID,
		"--workspace-folder",
		workspaceFolder,
	}

	if shell == "" {
		return append(args, "/VimRun.sh")
	}

	return append(args, shell)
}

// Assemble the arguments for `devcontainer exec` during `devcontainer.vim start`
//
// Args:
//   - containerID: Container ID
//   - workspaceFolder: Workspace folder path
//   - vimFileName: Filename of vim/nvim transferred to the container
//   - useSystemVim: If true, use vim/nvim installed on the system
//
// Return:
//
//	Array of command line arguments used for `devcontainer exec`
func devcontainerStartVimArgs(containerID string, workspaceFolder string, vimFileName string, tmuxFileName string, sendToTCP string, containerArch string, useSystemVim bool, useSystemTmux bool, noTmux bool, shell string, configDirForDevcontainer string) ([]string, error) {
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
	vimLaunchScript := filepath.Join(configDirForDevcontainer, "VimRun.sh")
	os.RemoveAll(vimLaunchScript)
	err = os.WriteFile(vimLaunchScript, []byte(vimRunScript), 0766)
	if err != nil {
		return nil, err
	}

	docker.Cp("Vim launch script", vimLaunchScript, containerID, "/VimRun.sh")

	return buildDevcontainerStartVimExecArgs(containerID, workspaceFolder, shell), nil
}
