//go:build darwin

package tools

const devcontainerFileName = "devcontainer"

// Download URL for devcontainer-cli
const downloadURLDevcontainersCliPattern = "https://github.com/mikoto2000/devcontainers-cli/releases/download/{{ .TagName }}/devcontainer-darwin-{{ .Arch }}-{{ .TagName }}"
