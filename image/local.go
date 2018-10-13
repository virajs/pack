package image

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	dockercli "github.com/docker/docker/client"
	"github.com/pkg/errors"
)

type local struct {
	RepoName string
	Docker   Docker
	Inspect  types.ImageInspect
	Stdout   io.Writer
}

func (f *Factory) NewLocal(repoName string, pull bool) (Image2, error) {
	if pull {
		f.Log.Printf("Pulling image '%s'\n", repoName)
		if err := f.Docker.PullImage(repoName); err != nil {
			return nil, fmt.Errorf("failed to pull image '%s' : %s", repoName, err)
		}
	}

	inspect, _, err := f.Docker.ImageInspectWithRaw(context.Background(), repoName)
	if err != nil && !dockercli.IsErrNotFound(err) {
		return nil, errors.Wrap(err, "analyze read previous image config")
	}

	return &local{
		Docker:   f.Docker,
		RepoName: repoName,
		Inspect:  inspect,
		Stdout:   f.Stdout,
	}, nil
}

func (l *local) Label(key string) (string, error) {
	if l.Inspect.Config == nil {
		return "", fmt.Errorf("failed to get label, image '%s' does not exist", l.RepoName)
	}
	labels := l.Inspect.Config.Labels
	return labels[key], nil
}

func (l *local) Name() string {
	return l.RepoName
}

func (*local) Rebase(string, Image2) error {
	panic("implement me")
}

func (l *local) SetLabel(key, val string) error {
	if l.Inspect.Config == nil {
		return fmt.Errorf("failed to set label, image '%s' does not exist", l.RepoName)
	}
	l.Inspect.Config.Labels[key] = val
	return nil
}

func (*local) TopLayer() (string, error) {
	panic("implement me")
}

func (l *local) Save() (string, error) {
	dockerFile := "FROM scratch\n"
	if l.Inspect.Config != nil {
		for k, v := range l.Inspect.Config.Labels {
			dockerFile += fmt.Sprintf("LABEL %s=%s\n", k, v)
		}
	}

	res, err := cli.ImageBuild(ctx, r2, dockertypes.ImageBuildOptions{Tags: []string{repoName}})
	if err != nil {
		return errors.Wrap(err, "image build")
	}
	defer res.Body.Close()
	if _, err := parseImageBuildBody(res.Body, l.Stdout); err != nil {
		return errors.Wrap(err, "image build")
	}
	res.Body.Close()
	return nil
}
