//go:build windows

package tools

const devcontainerFileName = "devcontainer.exe"

// Download URL for devcontainer-cli
const downloadURLDevcontainersCliPattern = "https://github.com/mikoto2000/devcontainers-cli/releases/download/{{ .TagName }}/devcontainer-windows-x64-{{ .TagName }}.exe"
