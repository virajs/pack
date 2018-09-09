package pack

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/buildpack/packs"
	"github.com/buildpack/packs/img"
	dockertypes "github.com/docker/docker/api/types"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func readImage(repoName string, useDaemon bool) (v1.Image, error) {
	repoStore, err := repoStore(repoName, useDaemon)
	if err != nil {
		return nil, err
	}

	origImage, err := repoStore.Image()
	if err != nil {
		// Assume error is due to non-existent image
		return nil, nil
	}
	if _, err := origImage.RawManifest(); err != nil {
		// Assume error is due to non-existent image
		// This is necessary for registries
		return nil, nil
	}

	return origImage, nil
}

func repoStore(repoName string, useDaemon bool) (img.Store, error) {
	newRepoStore := img.NewRegistry
	if useDaemon {
		newRepoStore = img.NewDaemon
	}
	repoStore, err := newRepoStore(repoName)
	if err != nil {
		return nil, packs.FailErr(err, "access", repoName)
	}
	return repoStore, nil
}

func singleFileTar(path, contents string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: path,
		Mode: 0666,
		Size: int64(len(contents)),
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(contents)); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return bytes.NewReader(buf.Bytes()), nil
}

type DockerImageBuild interface {
	ImageBuild(ctx context.Context, buildContext io.Reader, options dockertypes.ImageBuildOptions) (dockertypes.ImageBuildResponse, error)
}

func dockerBuild(cli DockerImageBuild, dockerfile string, out io.Writer) (string, error) {
	tr, err := singleFileTar("Dockerfile", dockerfile)
	if err != nil {
		return "", err
	}
	res, err := cli.ImageBuild(context.Background(), tr, dockertypes.ImageBuildOptions{})
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	jr := json.NewDecoder(res.Body)
	var id string
	var obj struct {
		Stream string `json:"stream"`
		Aux    struct {
			ID string `json:"ID"`
		} `json:"aux"`
	}
	for {
		err := jr.Decode(&obj)
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}
		if obj.Aux.ID != "" {
			id = obj.Aux.ID
		}
		if txt := strings.TrimSpace(obj.Stream); txt != "" {
			fmt.Fprintln(out, txt)
		}
	}

	return id, nil
}
