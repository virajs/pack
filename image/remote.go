package image

import (
	"fmt"

	"github.com/buildpack/lifecycle/img"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
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

func (r *remote) Rebase(baseTopLayer string, newBase Image2) error {
	newBaseRemote, ok := newBase.(*remote)
	if !ok {
		return errors.New("expected new base to be a remote image")
	}

	oldBase := &subImage{img: r.Image, topSHA: baseTopLayer}
	newImage, err := mutate.Rebase(r.Image, oldBase, newBaseRemote.Image, &mutate.RebaseOptions{})
	if err != nil {
		return errors.Wrap(err, "rebase")
	}
	r.Image = newImage
	return nil
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

type subImage struct {
	img    v1.Image
	topSHA string
}

func (si *subImage) Layers() ([]v1.Layer, error) {
	all, err := si.img.Layers()
	if err != nil {
		return nil, err
	}
	for i, l := range all {
		d, err := l.DiffID()
		if err != nil {
			return nil, err
		}
		if d.String() == si.topSHA {
			return all[:i+1], nil
		}
	}
	return nil, errors.New("could not find base layer in image")
}
func (si *subImage) BlobSet() (map[v1.Hash]struct{}, error)  { panic("Not Implemented") }
func (si *subImage) MediaType() (types.MediaType, error)     { panic("Not Implemented") }
func (si *subImage) ConfigName() (v1.Hash, error)            { panic("Not Implemented") }
func (si *subImage) ConfigFile() (*v1.ConfigFile, error)     { panic("Not Implemented") }
func (si *subImage) RawConfigFile() ([]byte, error)          { panic("Not Implemented") }
func (si *subImage) Digest() (v1.Hash, error)                { panic("Not Implemented") }
func (si *subImage) Manifest() (*v1.Manifest, error)         { panic("Not Implemented") }
func (si *subImage) RawManifest() ([]byte, error)            { panic("Not Implemented") }
func (si *subImage) LayerByDigest(v1.Hash) (v1.Layer, error) { panic("Not Implemented") }
func (si *subImage) LayerByDiffID(v1.Hash) (v1.Layer, error) { panic("Not Implemented") }
