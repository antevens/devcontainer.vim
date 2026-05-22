package util

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsExistsCommandOk(t *testing.T) {
	got := IsExistsCommand("ls")
	want := true
	if got != want {
		t.Fatalf("want %v, but %v:", want, got)
	}
}

func TestIsExistsCommandNg(t *testing.T) {
	got := IsExistsCommand("noExistsCommand")
	want := false
	if got != want {
		t.Fatalf("want %v, but %v:", want, got)
	}
}

func CreateTestCreateDirectorySuccessPath() (string, error) {
	tempDir := os.TempDir()
	testDirectory, err := os.MkdirTemp(tempDir, "devcontainer.vim-testdir")
	return testDirectory, err
}

func CreateTestCreateDirectoryFailedPath() (string, error) {
	return "/usr/bin", nil
}

func TestIsExistsTrue(t *testing.T) {
	want := true

	// Check for existence of an existing file
	got := IsExists("util_test.go")

	if got != want {
		t.Fatalf("want %v, but %v:", want, got)
	}

}

func TestIsExistsFalse(t *testing.T) {
	want := false

	// Check for existence of a non-existing file
	got := IsExists("noExistsFile")

	if got != want {
		t.Fatalf("want %v, but %v:", want, got)
	}

}

func pathFuncHello() (string, error) {
	return "Hello", nil
}

func pathFuncFailed() (string, error) {
	return "", errors.New("failed!")
}

func TestGetConfigDirectorySuccess(t *testing.T) {
	want := "Hello/success"
	// Check for existence of a non-existing file
	got, err := CreateConfigDirectory(pathFuncHello, "success")
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	defer func() {
		os.RemoveAll(filepath.Dir(got))
	}()

	_, err = os.Stat(want)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestGetConfigDirectoryFailed(t *testing.T) {
	want := "Hello/failed"
	// Check for existence of a non-existing file
	got, err := CreateConfigDirectory(pathFuncFailed, "failed")
	if err == nil {
		t.Fatalf("not return error got: %s", got)
	}

	defer func() {
		os.RemoveAll(filepath.Dir(got))
	}()

	_, err = os.Stat(want)
	if err == nil {
		t.Fatalf("not return error found: %s", got)
	}
}

func TestCreateCacheDirectorySuccess(t *testing.T) {
	wantBase := "Hello/success"
	// Check for existence of a non-existing file
	gotAppCacheDir, _, _, _, err := CreateCacheDirectory(pathFuncHello, "success")
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	defer func() {
		os.RemoveAll(filepath.Dir(gotAppCacheDir))
	}()

	_, err = os.Stat(filepath.Join(wantBase))
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	_, err = os.Stat(filepath.Join(wantBase, "bin"))
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	_, err = os.Stat(filepath.Join(wantBase, "config", "docker"))
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	_, err = os.Stat(filepath.Join(wantBase, "config", "devcontainer"))
	if err != nil {
		t.Fatalf("error: %s", err)
	}
}

func TestAddExecutePermission(t *testing.T) {
	target := "TestAddExecutePermission"
	err := os.Mkdir(target, 0644)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	defer func() {
		os.RemoveAll(target)
	}()

	err = AddExecutePermission(target)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	fileInfo, err := os.Stat(target)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	want := "drwxr-xr-x"
	got := fileInfo.Mode().String()
	if got != want {
		t.Fatalf("error: want %s but got %s", got, want)
	}
}

func TestParseJwcc(t *testing.T) {
	target := "test/resource/TestParseJwcc.json"
	jsonBytes, err := ParseJwcc(target)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	var unmarshaledJson map[string]interface{}
	json.Unmarshal(jsonBytes, &unmarshaledJson)

	want := "test_value"
	got := unmarshaledJson["test_key"]

	if got != want {
		t.Fatalf("error: want %s but got %s", got, want)
	}
}

func TestCreateConfigFileForDevcontainer(t *testing.T) {
	configDirForDevcontainer := "test/resource/config"
	workspaceFolder := "test/resource/TestCreateConfigFileForDevcontainer"
	configFilePath := "test/resource/TestCreateConfigFileForDevcontainer/.devcontainer/devcontainer.json"
	additionalConfigFilePath := "test/resource/TestCreateConfigFileForDevcontainer/.devcontainer/devcontainer.vim.json"

	mergedConfigFilePath, err := CreateConfigFileForDevcontainer(configDirForDevcontainer, workspaceFolder, configFilePath, additionalConfigFilePath)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	expectedDir, err := GetConfigDir(configDirForDevcontainer, workspaceFolder)
	if err != nil {
		t.Fatalf("error getting config dir: %s", err)
	}

	defer func() {
		os.RemoveAll(expectedDir)
	}()

	// An MD5 hashed folder is created under the config directory
	if !IsExists(expectedDir) {
		t.Fatalf("config directory not found: %s", expectedDir)
	}

	if !IsExists(filepath.Join(expectedDir, "devcontainer.json")) {
		t.Fatalf("config file not found in: %s", expectedDir)
	}

	bytes, err := os.ReadFile(mergedConfigFilePath)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	var unmarshaledJson map[string]interface{}
	json.Unmarshal(bytes, &unmarshaledJson)

	// Base JSON values can be retrieved
	want := unmarshaledJson["name"]
	got := "test_name"
	if want != got {
		t.Fatalf("error: want %s, but got %s", want, got)
	}

	// Additional JSON values can also be retrieved
	want2 := unmarshaledJson["additional_key"]
	got2 := "additional_value"
	if want != got {
		t.Fatalf("error: want %s, but got %s", want2, got2)
	}

}

func TestGetConfigDir(t *testing.T) {
	configDir := "test/resource/TestGetConfigDir/config"
	workspaceFolder := "test/resource/TestGetConfigDir"
	got, err := GetConfigDir(configDir, workspaceFolder)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	absPath, _ := filepath.Abs(workspaceFolder)
	hash := md5.Sum([]byte(absPath))
	hashStr := hex.EncodeToString(hash[:])
	want := filepath.Join(configDir, hashStr)

	if want != got {
		t.Fatalf("error: want %s, but got %s", want, got)
	}
}

func TestIsWsl(t *testing.T) {
	os.Setenv("WSL_DISTRO_NAME", "")
	got := IsWsl()
	if got != true {
		t.Fatalf("error: want true, but got false")
	}

	os.Unsetenv("WSL_DISTRO_NAME")
	got = IsWsl()
	if got != false {
		t.Fatalf("error: want false, but got true")
	}
}

func TestCreateFileWithContents(t *testing.T) {
	file := "test/TestCreateFileWithContents"
	contents := "testing"

	err := CreateFileWithContents(file, contents, 0755)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	defer os.RemoveAll(file)

	// Check for file existence
	fileInfo, err := os.Stat(file)
	if err != nil {
		t.Fatalf("error: %s", err)
	}

	// Is the mode as set?
	wantFileModeString := "-rwxr-xr-x"
	gotFileModeString := fileInfo.Mode().String()
	if wantFileModeString != gotFileModeString {
		t.Fatalf("error: want %s, but got %s", wantFileModeString, gotFileModeString)
	}

	// Is the content as set?
	bytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if contents != string(bytes) {
		t.Fatalf("error: want %s, but got %s", contents, string(bytes))
	}

}
