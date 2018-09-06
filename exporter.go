package pack

import (
	"io/ioutil"
	"os"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/packs"
	"github.com/buildpack/packs/img"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func export(group lifecycle.BuildpackGroup, launchDir, repoName, stackName string, useDaemon, useDaemonStack bool) (string, error) {
	var origImage v1.Image
	if !useDaemon {
		var err error
		origImage, err = readImage(repoName, useDaemon)
		if err != nil {
			return "", err
		}
	}

	stackImage, err := readImage(stackName, useDaemonStack)
	if err != nil || stackImage == nil {
		return "", packs.FailErr(err, "get image for", stackName)
	}

	var repoStore img.Store
	if useDaemon {
		repoStore, err = img.NewDaemon(repoName)
	} else {
		repoStore, err = img.NewRegistry(repoName)
	}
	if err != nil {
		return "", packs.FailErr(err, "access", repoName)
	}

	tmpDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return "", packs.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(tmpDir)

	exporter := &lifecycle.Exporter{
		Buildpacks: group.Buildpacks,
		TmpDir:     tmpDir,
		Out:        os.Stdout,
		Err:        os.Stderr,
	}
	newImage, err := exporter.Export(
		launchDir,
		stackImage,
		origImage,
	)
	if err != nil {
		return "", packs.FailErrCode(err, packs.CodeFailedBuild)
	}

	if err := repoStore.Write(newImage); err != nil {
		return "", packs.FailErrCode(err, packs.CodeFailedUpdate, "write")
	}

	sha, err := newImage.Digest()
	if err != nil {
		return "", packs.FailErr(err, "calculating image digest")
	}

	return sha.String(), nil
}
