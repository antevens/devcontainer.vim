package docker

import (
	"encoding/json"
)

// Execution result schema of the `docker ps --format json` command
//
// Example:
//
//	{
//	}
type PsCommandResult struct {
	ID string `json:"ID"`
}

func GetID(psCommandResult string) (string, error) {
	result, err := UnmarshalPsCommandResult([]byte(psCommandResult))
	if err != nil {
		return "", err
	}

	return result.ID, nil
}

func UnmarshalPsCommandResult(data []byte) (PsCommandResult, error) {
	var result PsCommandResult

	err := json.Unmarshal(data, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}
