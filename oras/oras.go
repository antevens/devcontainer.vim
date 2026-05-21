package oras

import (
	"context"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
)

func Pull(id string, tagName string, destDir string) error {

	// remote repository settings
	src, err := remote.NewRepository(id)
	if err != nil {
		return err
	}

	// Create file store
	dst, err := file.New(destDir)
	if err != nil {
		return err
	}
	ctx := context.Background()

	_, err = oras.Copy(ctx, src, tagName, dst, tagName, oras.DefaultCopyOptions)
	if err != nil {
		return err
	}

	return nil
}
