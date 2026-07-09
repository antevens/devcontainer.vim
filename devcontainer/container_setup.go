package devcontainer

import (
	"encoding/json"
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
func transferVimFiles(containerID, containerHome, configDir, vimrc string, noCdr bool, port int, isNvim bool, dereferencedMounts []map[string]interface{}) (string, error) {
	// Transfer Vim-related files (SendToTcp.vim and additional vimrc)
	sendToTCP, err := tools.CreateSendToTCP(configDir, port, noCdr, isNvim)
	if err != nil {
		return "", err
	}

	// Transfer SendToTcp.vim to the container
	err = docker.Cp("SendToTcp.vim", sendToTCP, containerID, "/"+filepath.Base(sendToTCP))
	if err != nil {
		return sendToTCP, err
	}

	// Transfer vimrc to the container
	err = docker.Cp("vimrc", vimrc, containerID, "/vimrc")
	if err != nil {
		return sendToTCP, err
	}

	// Transfer microk8s kubeconfig if available on the host.
	// Done before dereferenced mounts so an explicit user mount can override it.
	if err := transferMicroK8sConfig(containerID, containerHome); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to transfer microk8s config: %v\n", err)
	}

	for _, mount := range dereferencedMounts {
		sourceRaw, okSource := mount["source"].(string)
		targetRaw, okTarget := mount["target"].(string)

		if !okSource || !okTarget {
			fmt.Fprintf(os.Stderr, "Warning: invalid dereferenced mount format: %v\n", mount)
			continue
		}

		err = transferDereferencedMount(containerID, containerHome, sourceRaw, targetRaw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to transfer dereferenced mount %s -> %s: %v\n", sourceRaw, targetRaw, err)
		}
	}

	// Set up Claude response logging after mounts so our settings.local.json
	// is written last and won't be overwritten by any .claude dereferenced mount.
	if err := setupClaudeLogging(containerID, containerHome); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to set up Claude response logging: %v\n", err)
	}

	return sendToTCP, nil
}

// transferMicroK8sConfig copies the kubeconfig produced by `microk8s config`
// into the container as $HOME/.kube/config. It silently skips when microk8s
// is not installed on the host or is not yet initialised/running.
func transferMicroK8sConfig(containerID, containerHome string) error {
	if _, err := exec.LookPath("microk8s"); err != nil {
		return nil
	}

	kubeConfigBytes, err := exec.Command("microk8s", "config").Output()
	if err != nil {
		// microk8s present but not running or not yet initialised — skip.
		return nil
	}

	tmpFile, err := os.CreateTemp("", "microk8s-config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp file for microk8s config: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(kubeConfigBytes); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write microk8s config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close microk8s config temp file: %w", err)
	}

	kubeDir := containerHome + "/.kube"
	if _, err := docker.Exec(containerID, "mkdir", "-p", kubeDir); err != nil {
		return fmt.Errorf("create .kube directory in container: %w", err)
	}

	return docker.Cp("microk8s-config", tmpFile.Name(), containerID, kubeDir+"/config")
}

// isVimrcTarget returns true when target is a Vim/Neovim init file that
// should be treated as a vimrc (merged rather than blindly overwritten).
func isVimrcTarget(target string) bool {
	base := filepath.Base(target)
	switch base {
	case ".vimrc", "vimrc":
		return true
	case "init.vim":
		return strings.Contains(target, "/.vim/") || strings.Contains(target, "/nvim/")
	}
	return false
}

// mergeVimrcContent concatenates sourceFile content followed by additionalFile
// content and returns the combined bytes.
func mergeVimrcContent(sourceFile, additionalFile string) ([]byte, error) {
	source, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, fmt.Errorf("read source vimrc %s: %w", sourceFile, err)
	}
	additional, err := os.ReadFile(additionalFile)
	if err != nil {
		return nil, fmt.Errorf("read additional vimrc %s: %w", additionalFile, err)
	}
	return append(source, additional...), nil
}

func transferDereferencedMount(containerID string, containerHome string, sourceRaw string, targetRaw string) error {
	sourceExpanded, err := util.ExtractShellVariables(strings.ReplaceAll(sourceRaw, "${localEnv:HOME}", "$HOME"))
	if err != nil {
		sourceExpanded = sourceRaw
	}
	sourceExpanded = strings.TrimSpace(sourceExpanded)

	targetExpanded := strings.ReplaceAll(targetRaw, "{{ remoteEnv:HOME }}", containerHome)

	if _, err := os.Stat(sourceExpanded); os.IsNotExist(err) {
		return nil
	}

	realPath, err := filepath.EvalSymlinks(sourceExpanded)
	if err != nil {
		return err
	}

	fileInfo, err := os.Stat(realPath)
	if err != nil {
		return err
	}

	if fileInfo.IsDir() {
		tmpDir, err := os.MkdirTemp("", "devcontainer-vim-transfer-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)

		dirName := filepath.Base(realPath)
		tmpSrc := filepath.Join(tmpDir, dirName)
		err = util.CopyDirDereference(realPath, tmpSrc)
		if err != nil {
			return err
		}

		return docker.Cp("derefMountDir", tmpSrc+"/.", containerID, targetExpanded)
	}

	return docker.Cp("derefMountFile", realPath, containerID, targetExpanded)
}

// claudeLogHookScript is written to ~/.claude/hooks/log-response.sh inside
// the container. It appends the last assistant response from the Claude Code
// transcript to .claude.log in the workspace directory.
const claudeLogHookScript = `#!/usr/bin/env bash
set -euo pipefail

input=$(cat)

command -v jq >/dev/null 2>&1 || exit 0

transcript_path=$(printf '%s' "${input}" | jq -r '.transcript_path // empty')
cwd=$(printf '%s' "${input}" | jq -r '.cwd // empty')

[[ -n "${transcript_path}" && -n "${cwd}" && -f "${transcript_path}" ]] || exit 0

last_response=""
last_response=$(jq -rs \
  '[.[] | select(.role == "assistant")] | last | .content[]? | select(.type == "text") | .text' \
  "${transcript_path}" 2>/dev/null) || last_response=""

[[ -n "${last_response}" ]] || exit 0

{
  printf '=== %s ===\n' "$(date -Iseconds)"
  printf '%s\n\n' "${last_response}"
} >> "${cwd}/.claude.log"
`

// mergeClaudeStopHook parses existing settings.local.json bytes (may be nil
// or empty) and adds hookCmd to the Stop hook list if not already present.
// Returns the merged JSON with a trailing newline.
func mergeClaudeStopHook(existing []byte, hookCmd string) ([]byte, error) {
	var settings map[string]interface{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &settings); err != nil {
			settings = make(map[string]interface{})
		}
	} else {
		settings = make(map[string]interface{})
	}

	hooksMap, _ := settings["hooks"].(map[string]interface{})
	if hooksMap == nil {
		hooksMap = make(map[string]interface{})
	}
	stopHooks, _ := hooksMap["Stop"].([]interface{})
	for _, h := range stopHooks {
		if m, ok := h.(map[string]interface{}); ok {
			if cmd, ok := m["command"].(string); ok && strings.Contains(cmd, "log-response.sh") {
				// Already present — return the original bytes unchanged.
				out, err := json.MarshalIndent(settings, "", "  ")
				if err != nil {
					return nil, err
				}
				return append(out, '\n'), nil
			}
		}
	}
	stopHooks = append(stopHooks, map[string]interface{}{
		"type":    "command",
		"command": hookCmd,
	})
	hooksMap["Stop"] = stopHooks
	settings["hooks"] = hooksMap

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// setupClaudeLogging installs a Claude Code Stop hook that appends every
// assistant response to .claude.log in the active workspace. It writes the
// hook script to ~/.claude/hooks/log-response.sh and merges the hook entry
// into ~/.claude/settings.local.json, preserving any existing entries.
func setupClaudeLogging(containerID, containerHome string) error {
	hookDir := containerHome + "/.claude/hooks"
	hookPath := hookDir + "/log-response.sh"
	settingsPath := containerHome + "/.claude/settings.local.json"

	// Create hooks directory.
	if _, err := docker.Exec(containerID, "mkdir", "-p", hookDir); err != nil {
		return fmt.Errorf("create claude hooks directory: %w", err)
	}

	// Write hook script via a temp file so we control permissions.
	hookFile, err := os.CreateTemp("", "claude-log-hook-*.sh")
	if err != nil {
		return fmt.Errorf("create temp hook script: %w", err)
	}
	defer os.Remove(hookFile.Name())
	if _, err := hookFile.WriteString(claudeLogHookScript); err != nil {
		hookFile.Close()
		return fmt.Errorf("write hook script: %w", err)
	}
	if err := hookFile.Close(); err != nil {
		return fmt.Errorf("close hook script: %w", err)
	}
	if err := os.Chmod(hookFile.Name(), 0755); err != nil {
		return fmt.Errorf("chmod hook script: %w", err)
	}
	if err := docker.Cp("claude-log-hook", hookFile.Name(), containerID, hookPath); err != nil {
		return fmt.Errorf("copy hook script: %w", err)
	}

	// Read existing settings.local.json from the container without -t so we
	// don't get CRLF-contaminated output.
	existingBytes, _ := exec.Command(containerCommand, "exec", containerID, "cat", settingsPath).Output()

	settingsBytes, err := mergeClaudeStopHook(existingBytes, "bash "+hookPath)
	if err != nil {
		return fmt.Errorf("merge claude settings: %w", err)
	}

	settingsFile, err := os.CreateTemp("", "claude-settings-*.json")
	if err != nil {
		return fmt.Errorf("create temp claude settings: %w", err)
	}
	defer os.Remove(settingsFile.Name())
	if _, err := settingsFile.Write(settingsBytes); err != nil {
		settingsFile.Close()
		return fmt.Errorf("write claude settings: %w", err)
	}
	if err := settingsFile.Close(); err != nil {
		return fmt.Errorf("close claude settings: %w", err)
	}
	if err := docker.Cp("claude-settings", settingsFile.Name(), containerID, settingsPath); err != nil {
		return fmt.Errorf("copy claude settings: %w", err)
	}

	return nil
}

