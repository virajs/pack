package pack

import (
	"archive/tar"
	"bytes"
	"io"

	"github.com/buildpack/packs"
	"github.com/buildpack/packs/img"
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
