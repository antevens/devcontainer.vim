package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const containerCommand = "docker"

type ContainerNotFoundError struct {
	msg string
}

func (e *ContainerNotFoundError) Error() string {
	return e.msg
}

// Returns the container ID corresponding to the directory specified by workspaceFolder.
func GetContainerIDFromWorkspaceFolder(workspaceFolder string) (string, error) {

	// Search for the line containing `devcontainer.local_folder=${workspaceFolder}`

	workspaceFilderAbs, err := filepath.Abs(workspaceFolder)
	if err != nil {
		return "", err
	}

	psResult, err := Ps("label=devcontainer.local_folder=" + workspaceFilderAbs)
	if psResult == "" {
		return "", &ContainerNotFoundError{msg: "container not found."}
	}
	if err != nil {
		return "", err
	}

	id, err := GetID(psResult)
	if err != nil {
		return "", err
	}

	return id, nil
}

// Executes the `docker exec` command.
func Exec(containerID string, command ...string) (string, error) {

	dockerExecArgs := []string{"exec", "-t", containerID}
	dockerExecArgs = append(dockerExecArgs, command...)

	dockerExecCommand := exec.Command(containerCommand, dockerExecArgs...)
	stdout, err := dockerExecCommand.Output()
	return string(stdout), err
}

// Executes the `docker ps --format json` command.
func Ps(filter string) (string, error) {
	args := []string{"ps", "--format", "json"}
	if filter != "" {
		args = append(args, "--filter", filter)
	}
	dockerPsCommand := exec.Command(containerCommand, args...)
	stdout, err := dockerPsCommand.Output()
	return string(stdout), err
}

// Executes the `docker stop ${containerID}` command.
func Stop(containerID string) error {
	dockerStopCommand := exec.Command(containerCommand, "stop", containerID)
	err := dockerStopCommand.Start()
	return err
}

// Executes the `docker rm -f ${containerID}` command.
func Rm(containerID string) error {
	dockerRmCommand := exec.Command(containerCommand, "rm", "-f", containerID)
	err := dockerRmCommand.Start()
	return err
}

func Cp(tagForLog string, from string, containerID string, to string) error {
	resolvedFrom, err := filepath.EvalSymlinks(from)
	if err != nil {
		resolvedFrom = from
	}

	// Preserve the "/." suffix for docker cp semantics if it was provided
	if strings.HasSuffix(from, string(filepath.Separator)+".") || strings.HasSuffix(from, "/.") {
		if !strings.HasSuffix(resolvedFrom, string(filepath.Separator)+".") && !strings.HasSuffix(resolvedFrom, "/.") {
			resolvedFrom = resolvedFrom + "/."
		}
	}

	// Preemptively remove the destination inside the container ONLY if it is a symlink or regular file.
	// We specifically avoid rm -rf to prevent wiping out a bind-mounted directory on the host.
	exec.Command(containerCommand, "exec", "--user", "root", containerID, "rm", "-f", to).Run()

	dockerCpArgs := []string{"cp", resolvedFrom, containerID + ":" + to}
	fmt.Printf("Copy %s: `%s \"%s\"` ...", tagForLog, containerCommand, strings.Join(dockerCpArgs, "\" \""))
	copyResult, err := exec.Command(containerCommand, dockerCpArgs...).CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, "copy error.")
		fmt.Fprintln(os.Stderr, string(copyResult))
		return err
	}
	fmt.Printf(" done.\n")
	return nil
}
