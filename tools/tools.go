package tools

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mikoto2000/devcontainer.vim/v3/util"
)

type InstallerUseServices interface {
	GetLatestReleaseFromGitHub(owner string, repository string) (string, error)
	Download(downloadURL string, destPath string) error
}

type DefaultInstallerUseServices struct{}

func (s DefaultInstallerUseServices) GetLatestReleaseFromGitHub(owner string, repository string) (string, error) {
	return util.GetLatestReleaseFromGitHub(owner, repository)
}

func (s DefaultInstallerUseServices) Download(downloadURL string, destPath string) error {
	return download(downloadURL, destPath)
}

// Tool information
type Tool struct {
	FileName             string
	CalculateDownloadURL func(containerArch string) (string, error)
	installFunc          func(downloadFunc func(downloadURL string, destPath string) error, downloadURL string, filePath string, containerArch string) (string, error)
	DownloadFunc         func(downloadURL string, destPath string) error
}

// Execute tool installation
func (t Tool) Install(installDir string, containerArch string, override bool) (string, error) {

	// Normalize here as it may be called directly from tool download
	containerArch, err := util.NormalizeContainerArch(containerArch)
	if err != nil {
		return "", nil
	}

	// Assemble the tool's installation destination
	var fileName string
	if containerArch != "" {
		fileName = t.FileName + "_" + containerArch
	} else {
		fileName = t.FileName
	}
	filePath := filepath.Join(installDir, fileName)

	if util.IsExists(filePath) && !override {
		fmt.Printf("%s aleady exist, use this.\n", filePath)
		return filePath, nil
	} else {
		downloadURL, err := t.CalculateDownloadURL(containerArch)
		if err != nil {
			return "", err
		}
		return t.installFunc(t.DownloadFunc, downloadURL, filePath, containerArch)
	}
}

// Installation process for tools that can be installed by simple file placement.
//
// Downloads the file from downloadURL and places it in filePath.
func simpleInstall(downloadFunc func(downloadURL string, destPath string) error, downloadURL string, filePath string) (string, error) {

	// Download tool
	err := downloadFunc(downloadURL, filePath)
	if err != nil {
		return filePath, err
	}

	// Grant execution permission
	err = util.AddExecutePermission(filePath)
	if err != nil {
		return filePath, err
	}

	return filePath, nil
}

// Structure for progress display
type ProgressWriter struct {
	Total   int64
	Current int64
}

func (p *ProgressWriter) Write(data []byte) (int, error) {
	n := len(data)
	p.Current += int64(n)

	percentage := float64(p.Current) / float64(p.Total) * 100.0
	fmt.Printf("%6.2f%%", percentage)

	// Move the cursor back 7 characters
	fmt.Printf("\033[7D")

	return n, nil
}

// File download process.
//
// Downloads the file from downloadURL and places it in destPath.
func download(downloadURL string, destPath string) error {
	fmt.Printf("Download %s from %s ...", filepath.Base(destPath), downloadURL)

	// Send HTTP GET request
	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	size := resp.ContentLength

	// Create file
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	progress := &ProgressWriter{
		Total: size,
	}

	// Write response content to file
	_, err = io.Copy(out, io.TeeReader(resp.Body, progress))
	if err != nil {
		return err
	}

	fmt.Printf(" done. \n")

	return nil
}

// Install tools for run subcommand
func InstallRunTools(installDir string, nvim bool) (string, error) {
	var err error
	cdrPath, err := CDR(DefaultInstallerUseServices{}).Install(installDir, "", false)
	if err != nil {
		return cdrPath, err
	}
	return cdrPath, err
}

func InstallVim(installDir string, nvim bool, containerArch string) (string, error) {
	var vimPath string
	var err error
	if !nvim {
		vimPath, err = VIM(DefaultInstallerUseServices{}).Install(installDir, containerArch, false)
	} else {
		if runtime.GOOS == "darwin" && containerArch == "amd64" {
			// fallback to vim if AppImage does not work on M1 Mac with amd64 container
			vimPath, err = VIM(DefaultInstallerUseServices{}).Install(installDir, containerArch, false)
		} else {
			vimPath, err = NVIM(DefaultInstallerUseServices{}).Install(installDir, containerArch, false)
		}
	}
	return vimPath, err
}

func InstallTmux(installDir string, containerArch string) (string, error) {
	return Tmux(DefaultInstallerUseServices{}).Install(installDir, containerArch, false)
}

// Install tools for start subcommand
// Returns devcontainerPath, cdrPath, and error
func InstallStartTools(services InstallerUseServices, installDir string) (string, string, error) {
	var err error
	devcontainerPath, err := DEVCONTAINER(services).Install(installDir, "", false)
	if err != nil {
		return devcontainerPath, "", err
	}
	cdrPath, err := CDR(services).Install(installDir, "", false)
	if err != nil {
		return devcontainerPath, cdrPath, err
	}
	return devcontainerPath, cdrPath, nil
}

// Install tools for devcontainer subcommand
func InstallDevcontainerTools(installDir string) (string, error) {
	devcontainerPath, err := DEVCONTAINER(DefaultInstallerUseServices{}).Install(installDir, "", false)
	return devcontainerPath, err
}

// Install tools for Templates subcommand
func InstallTemplatesTools(installDir string) (string, error) {
	devcontainerPath, err := DEVCONTAINER(DefaultInstallerUseServices{}).Install(installDir, "", false)
	return devcontainerPath, err
}

// Install tools for Stop subcommand
func InstallStopTools(installDir string) (string, error) {
	devcontainerPath, err := DEVCONTAINER(DefaultInstallerUseServices{}).Install(installDir, "", false)
	return devcontainerPath, err
}

// Install tools for Down subcommand
func InstallDownTools(installDir string) (string, error) {
	devcontainerPath, err := DEVCONTAINER(DefaultInstallerUseServices{}).Install(installDir, "", false)
	return devcontainerPath, err
}

// SelfUpdate downloads the latest release of devcontainer.vim from GitHub and replaces the current binary
func SelfUpdate(services InstallerUseServices) error {
	// Get the latest release tag name from GitHub
	latestTagName, err := services.GetLatestReleaseFromGitHub("mikoto2000", "devcontainer.vim")
	if err != nil {
		return err
	}

	// Construct the download URL for the latest release
	downloadURL := fmt.Sprintf("https://github.com/mikoto2000/devcontainer.vim/releases/download/%s/devcontainer.vim-%s-%s", latestTagName, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		downloadURL = downloadURL + ".exe"
	}

	// Download the latest release
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}

	// Rename the current binary to avoid "text file busy" error
	tempPath := executablePath + ".old"
	err = os.Rename(executablePath, tempPath)
	if err != nil {
		return err
	}

	_, err = simpleInstall(services.Download, downloadURL, executablePath)
	if err != nil {
		// Restore the original binary if download fails
		os.Rename(tempPath, executablePath)
		return err
	}

	// Remove the old binary
	os.Remove(tempPath)

	fmt.Println("devcontainer.vim has been updated to the latest version.")
	return nil
}
