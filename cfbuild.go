package pack

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/buildpack/forge"
	"github.com/buildpack/forge/app"
	"github.com/buildpack/forge/engine"
	"github.com/buildpack/forge/engine/docker"
)

func CFBuild(appDir, stack, repoName string, publish bool) error {
	appDir, err := filepath.Abs(appDir)
	if err != nil {
		return err
	}
	tmpDir, err := ioutil.TempDir("", "pack.cf-build.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	dockerEngine, err := docker.New(&engine.EngineConfig{})
	if err != nil {
		return err
	}

	stager := forge.NewStager(dockerEngine)

	appTar, err := app.Tar(appDir, `^.+\.droplet$`, `^\..+\.cache$`)
	if err != nil {
		return err
	}

	// TODO make this not removed each time
	cache, err := os.Create(filepath.Join(tmpDir, "cache"))
	if err != nil {
		return err
	}

	droplet, err := stager.Stage(&forge.StageConfig{
		AppTar:     appTar,
		Cache:      cache,
		CacheEmpty: true, // TODO: cacheSize == 0,
		// BuildpackZips: buildpackZips,
		Stack:      stack,
		OutputPath: filepath.Join(tmpDir, "droplet.tgz"),
		// ForceDetect:   options.forceDetect,
		// Color:         color.GreenString,
		// AppConfig:     appConfig,
	})
	if err != nil {
		return err
	}

	fmt.Println("droplet:", droplet)

	return errors.New("NOT YET IMPLEMENTED")
}
