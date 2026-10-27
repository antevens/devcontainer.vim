package devcontainer

import (
	_ "embed"

	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mikoto2000/devcontainer.vim/v3/docker"
	"github.com/mikoto2000/devcontainer.vim/v3/dockercompose"
	"github.com/mikoto2000/devcontainer.vim/v3/tools"
	"github.com/mikoto2000/devcontainer.vim/v3/util"
)

//go:embed run_vim_system.template.sh
var runVimX8664System string

//go:embed run_vim_x86_64_AppImage.template.sh
var runVimX8664AppImage string

//go:embed run_vim_x86_64_static.template.sh
var runVimX8664Static string

//go:embed run_vim_aarch64.template.sh
var runVimAarch64 string

const containerCommand = "docker"

type UnknownTypeError struct {
	msg string
}

func (e *UnknownTypeError) Error() string {
	return e.msg
}

func hasNoCdrOption(args []string) bool {
	for _, arg := range args {
		if arg == "--nocdr" {
			return true
		}
	}
	return false
}

func Stop(args []string, devcontainerPath string, configDirForDevcontainer string) error {

	// Determine docker compose usage with `devcontainer read-configuration`

	// Use the end of the command line arguments as the value for `--workspace-folder`
	workspaceFolder := args[len(args)-1]
	stdout, _ := ReadConfiguration(devcontainerPath, "--workspace-folder", workspaceFolder)
	if stdout == "" {
		fmt.Printf("This directory is not a workspace for devcontainer: %s\n", workspaceFolder)
		return nil
	}

	// Check if `dockerComposeFile` is included
	// If included, the container is built with docker compose
	if strings.Contains(stdout, "dockerComposeFile") {

		// Get compose information with docker compose ps command
		dockerComposePsResultString, err := dockercompose.Ps(workspaceFolder)
		if err != nil {
			return err
		}
		if dockerComposePsResultString == "" {
			fmt.Println("devcontainer already downed.")
			return nil
		}

		// Since only the first line is needed, get only the first line
		dockerComposePsResultFirstItemString := strings.Split(dockerComposePsResultString, "\n")[0]

		// Get project name from docker compose ps command result
		projectName, err := dockercompose.GetProjectName(dockerComposePsResultFirstItemString)
		if err != nil {
			return err
		}

		// Execute docker compose stop using the project name
		fmt.Printf("Run `docker compose -p %s stop`(Async)\n", projectName)

		// Search for the storage directory of docker-compose.yaml
		dockerComposeFileDir, err := findDockerComposeFileDir(workspaceFolder)
		if err != nil {
			return err
		}

		// Record the current directory and move to dockerComposeFileDir
		currentDir, err := os.Getwd()
		if err != nil {
			return err
		}
		os.Chdir(dockerComposeFileDir)

		err = dockercompose.Stop(projectName)
		if err != nil {
			return err
		}

		// Return to the original current directory
		os.Chdir(currentDir)

	} else {
		// Search for the container corresponding to the workspace and get the ID
		containerID, err := docker.GetContainerIDFromWorkspaceFolder(workspaceFolder)
		if err != nil {
			return err
		}

		// Execute stop on the retrieved container
		fmt.Printf("Run `docker stop -f %s stop`(Async)\n", containerID)
		err = docker.Stop(containerID)
		if err != nil {
			return err
		}
	}
	return nil
}

func Down(args []string, devcontainerPath string, configDirForDevcontainer string) error {

	// Determine docker compose usage with `devcontainer read-configuration`

	// Use the end of the command line arguments as the value for `--workspace-folder`
	workspaceFolder := args[len(args)-1]
	stdout, _ := ReadConfiguration(devcontainerPath, "--workspace-folder", workspaceFolder)
	if stdout == "" {
		fmt.Printf("This directory is not a workspace for devcontainer: %s\n", workspaceFolder)
		return nil
	}

	// Check if `dockerComposeFile` is included
	// If included, the container is built with docker compose
	var configDir string
	if strings.Contains(stdout, "dockerComposeFile") {

		// Search for the storage directory of docker-compose.yaml
		dockerComposeFileDir, err := findDockerComposeFileDir(workspaceFolder)
		if err != nil {
			return err
		}

		// Record the current directory and move to dockerComposeFileDir
		currentDir, err := os.Getwd()
		if err != nil {
			return err
		}
		_, devcontainerJSONDir, err := findJSONInfo(workspaceFolder)
		if err != nil {
			return err
		}

		err = os.Chdir(filepath.Join(devcontainerJSONDir, dockerComposeFileDir))
		if err != nil {
			return err
		}

		// Get compose information with docker compose ps command
		dockerComposePsResultString, err := dockercompose.Ps(workspaceFolder)
		if err != nil {
			return err
		}
		if dockerComposePsResultString == "" {
			fmt.Println("devcontainer already downed.")
			return nil
		}

		// Since only the first line is needed, get only the first line
		dockerComposePsResultFirstItemString := strings.Split(dockerComposePsResultString, "\n")[0]

		// Get project name from docker compose ps command result
		projectName, err := dockercompose.GetProjectName(dockerComposePsResultFirstItemString)
		if err != nil {
			return err
		}

		// Execute docker compose down using the project name
		fmt.Printf("Run `docker compose -p %s down`(Async)\n", projectName)
		err = dockercompose.Down(projectName)
		if err != nil {
			return err
		}

		// Return to the original current directory
		err = os.Chdir(currentDir)
		if err != nil {
			return err
		}

		// Record the name of the configuration file storage directory for each container (record container ID) for pid file reference
		configDir, err = util.GetConfigDir(configDirForDevcontainer, workspaceFolder)
		if err != nil {
			return err
		}
	} else {
		// Search for the container corresponding to the workspace and get the ID
		containerID, err := docker.GetContainerIDFromWorkspaceFolder(workspaceFolder)
		if err != nil {
			return err
		}

		// Execute rm on the retrieved container
		fmt.Printf("Run `docker rm -f %s down`(Async)\n", containerID)
		err = docker.Rm(containerID)
		if err != nil {
			return err
		}

		// Record the name of the configuration file storage directory for each container (record container ID) for pid file reference
		configDir, err = util.GetConfigDir(configDirForDevcontainer, workspaceFolder)
		if err != nil {
			return err
		}
	}

	if !hasNoCdrOption(args) {
		// Stop clipboard-data-receiver
		pidFile := filepath.Join(configDir, "pid")
		fmt.Printf("Read PID file: %s\n", pidFile)
		pidStringBytes, err := os.ReadFile(pidFile)
		if err != nil {
			return err
		}
		pid, err := strconv.Atoi(string(pidStringBytes))
		if err != nil {
			return err
		}
		fmt.Printf("clipboard-data-receiver PID: %d\n", pid)
		tools.KillCdr(pid)
	}

	err := os.RemoveAll(configDir)
	if err != nil {
		return err
	}
	return nil
}

var runUserCommandsArgsPrefix = []string{"run-user-commands"}

// runUserCommandsArgs builds the argument list for
// `devcontainer run-user-commands`. All arguments except the last (the
// workspace folder) are forwarded verbatim. --prebuild is always injected so
// that updateContentCommand is re-run on every invocation; duplicates are
// silently dropped. The override config and workspace folder are appended last.
func runUserCommandsArgs(args []string, configFilePath string, workspaceFolder string) []string {
	userArgs := args[0 : len(args)-1]

	hasPrebuild := false
	for _, a := range userArgs {
		if a == "--prebuild" {
			hasPrebuild = true
			break
		}
	}
	if !hasPrebuild {
		userArgs = append([]string{"--prebuild"}, userArgs...)
	}

	userArgs = append(userArgs, "--override-config", configFilePath, "--workspace-folder", workspaceFolder)
	return append(runUserCommandsArgsPrefix, userArgs...)
}

// Update re-runs the container's updateContentCommand via
// `devcontainer run-user-commands --prebuild`, which always re-runs
// updateContentCommand even if it has executed before. This is the correct
// lifecycle hook for idempotent dependency installs (e.g. npm install -g).
func Update(args []string, devcontainerPath string, configFilePath string) error {
	// Use the end of the command line arguments as the value for `--workspace-folder`
	workspaceFolder := args[len(args)-1]

	// Confirm this is actually a devcontainer workspace before doing anything.
	stdout, _ := ReadConfiguration(devcontainerPath, "--workspace-folder", workspaceFolder)
	if stdout == "" {
		fmt.Printf("This directory is not a workspace for devcontainer: %s\n", workspaceFolder)
		return nil
	}

	// Stream output live and honour Ctrl-C, mirroring startVim.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	devcontainerArgs := runUserCommandsArgs(args, configFilePath, workspaceFolder)
	fmt.Printf("run container: `%s \"%s\"`\n", devcontainerPath, strings.Join(devcontainerArgs, "\" \""))

	runUserCommands := exec.CommandContext(ctx, devcontainerPath, devcontainerArgs...)
	runUserCommands.Stdin = os.Stdin
	runUserCommands.Stdout = os.Stdout
	runUserCommands.Stderr = os.Stderr

	if err := runUserCommands.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Update failed. Is the container running? Try `devcontainer.vim start %s` first.\n", workspaceFolder)
		return err
	}
	return nil
}

// Find and return the location/directory of devcontainer.json
func findJSONInfo(workspaceFolder string) (string, string, error) {
	// Record the current directory and move to workspaceFolder
	currentDir, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	defer os.Chdir(currentDir)

	err = os.Chdir(workspaceFolder)
	if err != nil {
		return "", "", err
	}

	// Get devcontainer.json
	var devcontainerJSONPath, devcontainerJSONDir string
	if util.IsExists(".devcontainer/devcontainer.json") {
		devcontainerJSONPath = ".devcontainer/devcontainer.json"
		devcontainerJSONDir = filepath.Dir(devcontainerJSONPath)
	} else if util.IsExists(".devcontainer.json") {
		devcontainerJSONPath = ".devcontainer.json"
		devcontainerJSONDir = filepath.Dir(devcontainerJSONPath)
	}

	devcontainerJSONPath, err = filepath.Abs(devcontainerJSONPath)
	if err != nil {
		return "", "", err
	}
	devcontainerJSONDir, err = filepath.Abs(devcontainerJSONDir)
	if err != nil {
		return "", "", err
	}

	return devcontainerJSONPath, devcontainerJSONDir, nil
}

// Returns the storage directory of docker-compose.yaml
func findDockerComposeFileDir(workspaceFolder string) (string, error) {
	// Get devcontainer.json
	devcontainerJSONPath, devcontainerJSONDir, err := findJSONInfo(workspaceFolder)
	if err != nil {
		return "", err
	}

	// Read devcontainer.json
	// fmt.Printf("devcontainerJSONPath directory: %s\n", devcontainerJSONPath)
	devcontainerJSONBytes, err := util.ParseJwcc(devcontainerJSONPath)
	if err != nil {
		return "", err
	}

	// Assemble the storage directory for docker-compose.yaml
	devcontainerJSON, err := UnmarshalDevcontainerJSON(devcontainerJSONBytes)
	if err != nil {
		return "", err
	}

	// Get the location of docker-compose.yaml while distinguishing between string and []string
	iDockerComposeFile := devcontainerJSON.DockerComposeFile
	fmt.Println(iDockerComposeFile)
	var dockerComposeFilePath string
	switch v := iDockerComposeFile.(type) {
	case string:
		dockerComposeFilePath = v
	case []interface{}:
		vv := v[0].(string)
		dockerComposeFilePath = filepath.Join(devcontainerJSONDir, vv)
	default:
		return "", &UnknownTypeError{msg: "Invalid docker compose file path type. Please open an issue on GitHub."}
	}
	dockerComposeFileDir := filepath.Dir(dockerComposeFilePath)

	fmt.Printf("dockerComposeFileDir directory: %s\n", dockerComposeFileDir)
	return dockerComposeFileDir, nil
}

func GetConfigurationFilePath(devcontainerFilePath string, workspaceFolder string) (string, error) {
	stdout, _ := ReadConfiguration(devcontainerFilePath, "--workspace-folder", workspaceFolder)
	return GetConfigFilePath(stdout)
}

func ReadConfiguration(devcontainerFilePath string, readConfiguration ...string) (string, error) {
	args := append([]string{"read-configuration"}, readConfiguration...)
	result, err := Execute(devcontainerFilePath, args...)
	if err != nil {
		return "", errors.New("devcontainer read-configuration failed. Please check if .devcontainer.json exists and if the docker engine is running.")
	}
	return result, err
}

func Templates(
	devcontainerFilePath string,
	workspaceFolder string,
	templateID string) (string, error) {
	// Use the end of the command line arguments as the value for `--workspace-folder`

	args := []string{"templates", "apply", "--template-id", templateID, "--workspace-folder", workspaceFolder}
	return ExecuteCombineOutput(devcontainerFilePath, args...)
}

func Execute(devcontainerFilePath string, args ...string) (string, error) {
	fmt.Printf("run devcontainer: `%s %s`\n", devcontainerFilePath, strings.Join(args, " "))
	cmd := exec.Command(devcontainerFilePath, args...)
	stdout, err := cmd.Output()
	return string(stdout), err
}

func ExecuteCombineOutput(devcontainerFilePath string, args ...string) (string, error) {
	fmt.Printf("run devcontainer: `%s %s`\n", devcontainerFilePath, strings.Join(args, " "))
	cmd := exec.Command(devcontainerFilePath, args...)
	stdout, err := cmd.CombinedOutput()
	return string(stdout), err
}

// Create the configuration file used when starting devcontainer.vim
// The configuration file is stored in the config directory within the devcontainer.vim cache,
// in a directory named after the md5 hash of the workspace folder path.
func CreateConfigFile(devcontainerPath string, workspaceFolder string, configDirForDevcontainer string) (string, []map[string]interface{}, error) {
	// Get devcontainer configuration file path
	configFilePath, err := GetConfigurationFilePath(devcontainerPath, workspaceFolder)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil, fmt.Errorf("configuration file not found: %w", err)
		}
		return "", nil, err
	}

	// Search for additional configuration file for devcontainer.vim
	configurationFileName := configFilePath[:len(configFilePath)-len(filepath.Ext(configFilePath))]
	additionalConfigurationFilePath := configurationFileName + ".vim.json"

	// Place JSON in the configuration management folder
	mergedConfigFilePath, dereferencedMounts, err := util.CreateConfigFileForDevcontainer(configDirForDevcontainer, workspaceFolder, configFilePath, additionalConfigurationFilePath)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return "", nil, fmt.Errorf("permission error: %w", err)
		}
		return "", nil, err
	}

	fmt.Printf("Use configuration file: `%s`", mergedConfigFilePath)

	return mergedConfigFilePath, dereferencedMounts, err
}
