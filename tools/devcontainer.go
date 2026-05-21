package tools

import (
	"runtime"
	"strings"
	"text/template"
)

// Tool information for devcontainer/cli
var DEVCONTAINER = func(services InstallerUseServices) Tool {

	return Tool{
		FileName: devcontainerFileName,
		CalculateDownloadURL: func(_ string) (string, error) {
			latestTagName, err := services.GetLatestReleaseFromGitHub("mikoto2000", "devcontainers-cli")
			if err != nil {
				return "", err
			}

			pattern := "pattern"
			tmpl, err := template.New(pattern).Parse(downloadURLDevcontainersCliPattern)
			if err != nil {
				return "", err
			}

			// Convert amd64 to x64 if GOARCH is amd64
			arch := runtime.GOARCH
			if arch == "amd64" {
				arch = "x64"
			}

			tmplParams := map[string]string{
				"TagName": latestTagName,
				"Arch":    arch,
			}
			var downloadURL strings.Builder
			err = tmpl.Execute(&downloadURL, tmplParams)
			if err != nil {
				return "", err
			}
			return downloadURL.String(), nil
		},
		installFunc: func(downloadFunc func(downloadURL string, destPath string) error, downloadURL string, filePath string, containerArch string) (string, error) {
			return simpleInstall(downloadFunc, downloadURL, filePath)
		},
		DownloadFunc: services.Download,
	}
}
