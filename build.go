package pack

import (
	"crypto/md5"
	"fmt"

	eng "github.com/buildpack/forge/engine"
	engdocker "github.com/buildpack/forge/engine/docker"
	forgeV3 "github.com/buildpack/forge/v3"
	"github.com/google/uuid"
)

func Build(appDir, detectImage, repoName string, publish bool) error {
	b := &Builder{
		AppDir:      appDir,
		DetectImage: detectImage,
		RepoName:    repoName,
		Publish:     publish,
	}
	if err := b.Init(); err != nil {
		return err
	}
	return b.Run()
}

type Builder struct {
	AppDir      string
	DetectImage string
	RepoName    string
	Publish     bool
	// Done by Init()
	Builder *forgeV3.Builder
}

func (b *Builder) Init() error {
	engine, err := engdocker.New(&eng.EngineConfig{})
	if err != nil {
		return err
	}

	appUUID := fmt.Sprintf("%x", md5.Sum([]byte(b.AppDir)))
	b.Builder, err = forgeV3.NewBuilder(engine, b.DetectImage, uuid.New().String(), appUUID)
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) Run() error {
	fmt.Println("*** PULL DETECT IMAGE:")
	waitFor(b.Builder.Pull(b.DetectImage))

	fmt.Println("*** COPY APP TO VOLUME:")
	tr, err := createTarReader(b.AppDir, "/launch/app")
	if err != nil {
		return err
	}
	if err := b.Builder.LaunchVolume.Upload(tr); err != nil {
		return err
	}

	fmt.Println("*** DETECTING:")
	group, err := b.Builder.Detect(b.DetectImage)
	if err != nil {
		return err
	}

	fmt.Println("*** ANALYZING:")
	if err := b.Builder.Analyze(b.RepoName, !b.Publish, group); err != nil {
		return err
	}

	fmt.Println("*** PULL BUILD IMAGE:")
	waitFor(b.Builder.Pull(group.BuildImage))

	fmt.Println("*** BUILDING:")
	if err := b.Builder.Build(group); err != nil {
		return err
	}

	if !b.Publish {
		fmt.Println("*** PULL RUN IMAGE:")
		waitFor(b.Builder.Pull(group.RunImage))
	}

	fmt.Println("*** EXPORTING:")
	imgSHA, err := b.Builder.Export(b.RepoName, !b.Publish, group)
	if err != nil {
		return err
	}

	if b.Publish {
		fmt.Printf("\n*** Image: %s@%s\n", b.RepoName, imgSHA)
	}

	return nil
}

func waitFor(c <-chan eng.Progress) {
	for {
		select {
		case _, ok := <-c:
			if !ok {
				return
			}
		}
	}
}
