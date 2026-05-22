package devcontainer

import (
	"encoding/json"
)

// Schema (part of) devcontainer.json
type DevcontainerJSON struct {
	DockerComposeFile interface{} `json:"dockerComposeFile"`
	Mounts            []Mount     `json:"mounts"`
}

type Mount struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func UnmarshalDevcontainerJSON(data []byte) (DevcontainerJSON, error) {
	var result DevcontainerJSON

	err := json.Unmarshal(data, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}
