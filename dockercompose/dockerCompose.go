package dockercompose

import (
	"os"
	"os/exec"
)

const containerCommand = "docker"

type PsCommandError struct {
	msg string
}

func (e *PsCommandError) Error() string {
	return e.msg
}

type StopCommandError struct {
	msg string
}

func (e *StopCommandError) Error() string {
	return e.msg
}

type DownCommandError struct {
	msg string
}

func (e *DownCommandError) Error() string {
	return e.msg
}

// Executes `docker compose ps --format json` and returns the result string.
func Ps(workspaceFolder string) (string, error) {

	// Remember current directory
	currentDirectory, err := os.Getwd()
	if err != nil {
		return "", &PsCommandError{msg: "Failed to get current directory"}
	}

	// Return to original directory
	defer func() error {
		err := os.Chdir(currentDirectory)
		if err != nil {
			return &PsCommandError{msg: "Failed to change directory"}
		}
		return nil
	}()

	// Move to workspace
	err = os.Chdir(workspaceFolder)
	if err != nil {
		return "", &PsCommandError{msg: "Failed to move to the workspace. Please check if the specified directory exists and if the permissions are correct."}
	}

	dockerComposePsCommand := exec.Command(containerCommand, "compose", "ps", "--all", "--format", "json")
	stdout, err := dockerComposePsCommand.Output()
	if err != nil {
		return "", &PsCommandError{msg: "Failed to execute docker compose ps command. Please check if docker is installed and if the docker engine is running."}
	}
	return string(stdout), err
}

// Executes `docker compose -p ${projectName} stop`.
func Stop(projectName string) error {
	dockerComposeStopCommand := exec.Command(containerCommand, "compose", "-p", projectName, "stop")
	err := dockerComposeStopCommand.Start()
	if err != nil {
		return &StopCommandError{msg: "Failed to execute docker compose stop command. Please check if docker is installed and if the docker engine is running."}
	}
	return nil
}

// Executes `docker compose -p ${projectName} down`.
func Down(projectName string) error {
	dockerComposeDownCommand := exec.Command(containerCommand, "compose", "-p", projectName, "down")
	err := dockerComposeDownCommand.Start()
	if err != nil {
		return &DownCommandError{msg: "Failed to execute docker compose down command. Please check if docker is installed and if the docker engine is running."}
	}
	return nil
}
