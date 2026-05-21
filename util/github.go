package util

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/v62/github"
)

/**
 * Returns the latest release tag name from the owner and repository name.
 */
func GetLatestReleaseFromGitHub(owner string, repository string) (string, error) {
	ctx := context.Background()
	client := github.NewClient(nil)

	release, _, err := client.Repositories.GetLatestRelease(ctx, owner, repository)
	if err != nil {
		message := fmt.Sprintf("Error getting latest release: %v", err)
		return "", errors.New(message)
	}

	return release.GetTagName(), nil
}
