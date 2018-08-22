package pack

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/buildpack/forge"
	"github.com/buildpack/forge/engine"
	"github.com/buildpack/forge/engine/docker"
)

func CFBuild(appDir, stack, repoName string, publish bool) error {
	appDir, err := filepath.Abs(appDir)
	if err != nil {
		return err
	}

	dockerEngine, err := docker.New(&engine.EngineConfig{})
	if err != nil {
		return err
	}

	stager := forge.NewStager(dockerEngine)
	droplet, err := stager.Stage(&forge.StageConfig{})
	if err != nil {
		return err
	}

	fmt.Println("droplet:", droplet)

	return errors.New("NOT YET IMPLEMENTED")
}
