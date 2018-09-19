package pack

import (
	"github.com/google/go-containerregistry/pkg/v1"
)

type Stack struct {
	BuildImage v1.Image
}

const defaultBuildRepo = "packs/build"

func DefaultStack() (Stack, error) {
	buildImage, err := readImage(defaultBuildRepo, true)
	if err != nil {
		return Stack{}, err
	}
	return Stack{
		BuildImage: buildImage,
	}, nil
}
