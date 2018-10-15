package image

import (
	"fmt"

	"github.com/buildpack/lifecycle/img"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"
	// dockertypes "github.com/docker/docker/api/types"
)

// type constError string
// func (e constError) Error() string { return string(e) }
// const NotFound = constError("image not found")

type remote struct {
	RepoName string
	Image    v1.Image
	// Docker   Docker
	// Inspect  types.ImageInspect
	// Stdout   io.Writer
	// FS       *fs.FS
}

func (f *Factory) NewRemote(repoName string) (Image2, error) {
	repoStore, err := img.NewRegistry(repoName)
	if err != nil {
		return nil, err
	}
	image, err := repoStore.Image()
	if err != nil {
		return nil, errors.New("connect to repo store")
	}

	return &remote{
		RepoName: repoName,
		Image:    image,
		// Inspect:  inspect,
		// Stdout:   f.Stdout,
		// FS:       f.FS,
	}, nil
}

func (r *remote) Label(key string) (string, error) {
	cfg, err := r.Image.ConfigFile()
	if err != nil || cfg == nil {
		return "", fmt.Errorf("failed to get label, image '%s' does not exist", r.RepoName)
	}
	labels := cfg.Config.Labels
	return labels[key], nil

}

func (r *remote) Name() string {
	return r.RepoName
}

func (*remote) Rebase(string, Image2) error {
	panic("implement me")
}

func (r *remote) SetLabel(key, val string) error {
	newImage, err := img.Label(r.Image, key, val)
	if err != nil {
		return errors.Wrap(err, "set metadata label")
	}
	r.Image = newImage
	return nil
}

func (*remote) TopLayer() (string, error) {
	panic("implement me")
}

func (r *remote) Save() (string, error) {
	repoStore, err := img.NewRegistry(r.RepoName)
	if err != nil {
		return "", err
	}
	if err := repoStore.Write(r.Image); err != nil {
		return "", err
	}
	return "TODO", nil
}
