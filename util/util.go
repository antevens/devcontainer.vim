package util

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Jeffail/gabs/v2"
	"github.com/tailscale/hujson"
)

const binDirName = "bin"
const configDirName = "config"

// Check if the command specified by command is in the PATH.
// Returns true if it is in the PATH, and false otherwise.
func IsExistsCommand(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

type GetDirFunc func() (string, error)

// Create and return the configuration directory used by devcontainer.vim.
func CreateConfigDirectory(pathFunc GetDirFunc, dirName string) (string, error) {
	var baseDir, err = pathFunc()
	if err != nil {
		return "", err
	}
	var appConfigDir = filepath.Join(baseDir, dirName)
	if err := os.MkdirAll(appConfigDir, 0766); err != nil {
		return "", err
	}
	return appConfigDir, nil
}

// Create and return the cache directory used by devcontainer.vim.
//
// Return values:
// 1. Cache directory for devcontainer.vim
// 2. Binary directory for devcontainer.vim
// 3. Configuration directory for docker
// 4. Configuration directory for devcontainer
func CreateCacheDirectory(pathFunc GetDirFunc, dirName string) (string, string, string, string, error) {
	var baseDir, err = pathFunc()
	if err != nil {
		return "", "", "", "", err
	}
	var appCacheDir = filepath.Join(baseDir, dirName)
	if err := os.MkdirAll(appCacheDir, 0766); err != nil {
		return "", "", "", "", err
	}
	var binDir = filepath.Join(baseDir, dirName, binDirName)
	if err := os.MkdirAll(binDir, 0766); err != nil {
		return appCacheDir, "", "", "", err
	}
	var configDir = filepath.Join(baseDir, dirName, configDirName)
	if err := os.MkdirAll(configDir, 0766); err != nil {
		return appCacheDir, binDir, "", "", err
	}
	// Create configuration directory for docker
	var configDirForDocker = filepath.Join(baseDir, dirName, configDirName, "docker")
	if err := os.MkdirAll(configDirForDocker, 0766); err != nil {
		return appCacheDir, binDir, "", "", err
	}
	// Create configuration directory for devcontainer
	var configDirForDevcontainer = filepath.Join(baseDir, dirName, configDirName, "devcontainer")
	if err := os.MkdirAll(configDirForDevcontainer, 0766); err != nil {
		return appCacheDir, binDir, configDir, "", err
	}
	return appCacheDir, binDir, configDirForDocker, configDirForDevcontainer, nil
}

func IsExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func AddExecutePermission(filePath string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	fileMode := fileInfo.Mode()
	err = os.Chmod(filePath, fileMode|0111)
	if err != nil {
		return err
	}

	return nil
}

// Merge the JSON specified by additionalConfigPath into the JSON specified by baseConfigPath and return the result
func readAndMergeConfig(baseConfigPath string, additionalConfigPath string) ([]byte, error) {

	// Parse the configuration file as JWCC and convert it to standard JSON
	parsedBaseJSON, err := ParseJwcc(baseConfigPath)
	if err != nil {
		return nil, err
	}

	// Re-parse the standard JSON using gabs
	parsedBaseJSONGrabContainer, err := gabs.ParseJSON(parsedBaseJSON)
	if err != nil {
		return nil, err
	}

	// Load additional configuration file for devcontainer.vim
	parsedAdditionalJSON, err := ParseJwcc(additionalConfigPath)
	if err != nil {
		return nil, err
	}

	// Re-parse the standard JSON using gabs
	parsedAdditionalJSONGrabContainer, err := gabs.ParseJSON(parsedAdditionalJSON)
	if err != nil {
		return nil, err
	}

	// Merge JSON using gabs
	parsedBaseJSONGrabContainer.Merge(parsedAdditionalJSONGrabContainer)

	// Return the content of the configuration file
	return parsedBaseJSONGrabContainer.Bytes(), nil
}

// Convert JWCC to standard JSON and return it as []byte
func ParseJwcc(jwccPath string) ([]byte, error) {
	// Read JWCC file
	jwccContentBytes, err := os.ReadFile(jwccPath)
	if err != nil {
		return []byte{}, err
	}

	// Parse JWCC and convert it to standard JSON
	parsedJSON, err := hujson.Parse(jwccContentBytes)
	if err != nil {
		return []byte{}, err
	}

	parsedJSON.Standardize()

	return parsedJSON.Pack(), nil
}

// Merge configFilePath and additionalConfigFilePath JSON,
// and store it in the configuration file storage directory within the devcontainer.vim cache directory.
// Return the path to the directory containing the created devcontainer.json.
func CreateConfigFileForDevcontainer(configDirForDevcontainer string, workspaceFolder string, configFilePath string, additionalConfigFilePath string) (string, error) {

	// Determine if merging is necessary and construct the final JSON content
	var configFileContent []byte
	var err error
	if IsExists(additionalConfigFilePath) {
		// Merge JSON
		configFileContent, err = readAndMergeConfig(configFilePath, additionalConfigFilePath)
	} else {
		// Use base configuration as is
		configFileContent, err = os.ReadFile(configFilePath)
	}
	if err != nil {
		return "", err
	}

	// Place JSON in the configuration management folder
	generateConfigDir, err := GetConfigDir(configDirForDevcontainer, workspaceFolder)
	if err != nil {
		return "", err
	}
	generateConfigFilePath := filepath.Join(generateConfigDir, "devcontainer.json")
	err = os.MkdirAll(generateConfigDir, 0777)
	if err != nil {
		return "", err
	}
	err = os.WriteFile(generateConfigFilePath, configFileContent, 0666)
	if err != nil {
		return "", err
	}
	return generateConfigFilePath, nil
}

// Calculate and return the storage directory for devcontainer.json for devcontainer.vim.
// Returns the directory `<devcontainer.vim cache directory>/config/<md5 hashed absolute path of workspaceFolder>`
func GetConfigDir(configDirForDevcontainer string, workspaceFolder string) (string, error) {
	workspaceFolderAbs, err := filepath.Abs(workspaceFolder)
	if err != nil {
		return "", err
	}
	workspaceFolderHash := md5.Sum([]byte(workspaceFolderAbs))

	workspaceFolderHashString := hex.EncodeToString(workspaceFolderHash[:])
	return filepath.Join(configDirForDevcontainer, workspaceFolderHashString), nil
}

// Determine if running on WSL
func IsWsl() bool {
	_, exists := os.LookupEnv("WSL_DISTRO_NAME")
	return exists
}

// Open with the associated application
func OpenFileWithDefaultApp(filePath string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", filePath) // macOS
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", filePath) // Windows
	default:
		cmd = exec.Command("xdg-open", filePath) // Linux
	}

	return cmd.Run()
}

func CreateFileWithContents(file string, content string, permission fs.FileMode) error {
	err := os.WriteFile(file, []byte(content), permission)
	if err != nil {
		return err
	}
	return nil
}

// Expand shell variables in the string and return it
func ExtractShellVariables(str string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		return "", errors.New("ExtractShellVariables no support windows")
	} else {
		cmd = exec.Command("sh", "-c", "echo "+str)
	}

	extractedStrBytes, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(extractedStrBytes), nil
}

func NormalizeContainerArch(containerArch string) (string, error) {
	if containerArch == "amd64" || containerArch == "x86_64" {
		return "amd64", nil
	} else if containerArch == "arm64" || containerArch == "aarch64" {
		return "aarch64", nil
	} else if containerArch == "" {
		return "", nil
	} else {
		return "", errors.New("Unknown Architecture")
	}
}

func RemoveEmptyString(input []string) []string {
	var result []string

	for _, v := range input {
		if strings.TrimSpace(v) != "" {
			result = append(result, v)
		}
	}

	return result
}
