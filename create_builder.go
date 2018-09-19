package pack

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/img"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type CreateBuilderArgs struct {
	RepoName  string
	Builder   BuilderConfig
	Stack     Stack
}

type BuilderConfig struct {
	Buildpacks []Buildpack                `toml:"buildpacks"`
	Groups     []lifecycle.BuildpackGroup `toml:"groups"`
}

type Buildpack struct {
	ID  string
	URI string
}

type BuilderFactory struct {
	DefaultStack Stack
	FS           FS
}

//go:generate mockgen -package mocks -destination mocks/fs.go github.com/buildpack/pack FS
type FS interface {
	CreateTarFile(tarFile, srcDir, tarDir string) error
}

func (f *BuilderFactory) Create(args CreateBuilderArgs) error {
	builderStore, err := repoStore(args.RepoName, true)
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir("", "create-builder")
	if err != nil {
		return err
	}
	defer os.Remove(tmpDir)

	orderTar, err := f.orderLayer(tmpDir, args.Builder.Groups)
	if err != nil {
		return err
	}
	builderImage, _, err := img.Append(args.Stack.BuildImage, orderTar)
	if err != nil {
		return err
	}
	for _, buildpack := range args.Builder.Buildpacks {
		tarFile, err := f.buildpackLayer(tmpDir, buildpack)
		if err != nil {
			return err
		}
		builderImage, _, err = img.Append(builderImage, tarFile)
		if err != nil {
			return err
		}
	}

	return builderStore.Write(builderImage)
}

type order struct {
	Groups []lifecycle.BuildpackGroup `toml:"groups"`
}

func (f *BuilderFactory) orderLayer(dest string, groups []lifecycle.BuildpackGroup) (layerTar string, err error) {
	buildpackDir := filepath.Join(dest, "buildpack")
	err = os.Mkdir(buildpackDir, 0755)
	if err != nil {
		return "", err
	}

	orderFile, err := os.Create(filepath.Join(buildpackDir, "order.toml"))
	if err != nil {
		return "", err
	}
	defer orderFile.Close()
	err = toml.NewEncoder(orderFile).Encode(order{Groups: groups})
	if err != nil {
		return "", err
	}
	layerTar = filepath.Join(dest, "order.tar")
	if err := f.FS.CreateTarFile(layerTar, buildpackDir, "/buildpacks"); err != nil {
		return "", err
	}
	return layerTar, nil
}

func (f *BuilderFactory) buildpackLayer(dest string, buildpack Buildpack) (layerTar string, err error) {
	dir := strings.TrimPrefix(buildpack.URI, "file://")
	var data struct {
		BP struct {
			ID      string `toml:"id"`
			Version string `toml:"version"`
		} `toml:"buildpack"`
	}
	_, err = toml.DecodeFile(filepath.Join(dir, "buildpack.toml"), &data)
	if err != nil {
		return "", errors.Wrapf(err, "reading buildpack.toml from buildpack: %s", filepath.Join(dir, "buildpack.toml"))
	}
	bp := data.BP
	if buildpack.ID != bp.ID {
		return "", fmt.Errorf("buildpack ids did not match: %s != %s", buildpack.ID, bp.ID)
	}
	if bp.Version == "" {
		return "", fmt.Errorf("buildpack.toml must provide version: %s", filepath.Join(dir, "buildpack.toml"))
	}
	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", buildpack.ID, bp.Version))
	if err := f.FS.CreateTarFile(tarFile, dir, filepath.Join("/buildpacks", buildpack.ID, bp.Version)); err != nil {
		return "", err
	}
	return tarFile, err
}
