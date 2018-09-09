package pack

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/packs"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func (b *BuildFlags) dockerBuildExport(group *lifecycle.BuildpackGroup, launchVolume, launchDir, repoName, stackName string) (string, error) {
	ctx := context.Background()
	image := stackName
	metadata := packs.BuildMetadata{
		RunImage: packs.RunImageMetadata{
			Name: stackName,
			SHA:  "TODO",
		},
		App:        packs.AppMetadata{},
		Config:     packs.ConfigMetadata{},
		Buildpacks: []packs.BuildpackMetadata{},
	}

	mvDir := func(image, name string) (string, error) {
		ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
			Image:      image,
			User:       "root",
			Entrypoint: []string{},
			Cmd:        []string{"bash", "-c", fmt.Sprintf(`mkdir -p "$(dirname /launch/%s)" && mv "/launch-volume/%s" "/launch/%s" && chown -R packs:packs "/launch/"`, name, name, name)},
		}, &container.HostConfig{
			Binds: []string{
				launchVolume + ":/launch-volume",
			},
		}, nil, "")
		if err != nil {
			return "", err
		}
		defer b.Cli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{Force: true})
		if err := b.runContainer(ctx, ctr.ID, ""); err != nil {
			return "", err
		}
		res, err := b.Cli.ContainerCommit(ctx, ctr.ID, dockertypes.ContainerCommitOptions{})
		if err != nil {
			return "", err
		}
		fmt.Println("ADD LAYER:", res.ID)
		return res.ID, nil
	}

	var err error
	image, err = mvDir(image, "app")
	if err != nil {
		return "", err
	}
	metadata.App.SHA = image

	image, err = mvDir(image, "config")
	if err != nil {
		return "", err
	}
	metadata.Config.SHA = image

	for _, buildpack := range group.Buildpacks {
		layers := make(map[string]packs.LayerMetadata)
		dirs, err := filepath.Glob(filepath.Join(launchDir, buildpack.ID, "*.toml"))
		if err != nil {
			return "", err
		}
		for _, tomlFile := range dirs {
			dir := strings.TrimSuffix(tomlFile, ".toml")
			name := filepath.Base(dir)
			if name == "launch" {
				continue
			}
			exists := true
			if _, err := os.Stat(dir); err != nil {
				if os.IsNotExist(err) {
					exists = false
				} else {
					return "", err
				}
			}
			dir, err = filepath.Rel(launchDir, dir)
			if err != nil {
				return "", err
			}
			if exists {
				image, err = mvDir(image, dir)
				if err != nil {
					return "", err
				}
			} else {
				// dockerFile += fmt.Sprintf("COPY --from=prev --chown=packs:packs /launch/%s /launch/%s\n", dir, dir)
				fmt.Println("TODO: Need to add dir from prev image:", dir)
				continue
			}

			var data interface{}
			if _, err := toml.DecodeFile(tomlFile, &data); err != nil {
				return "", err
			}
			layers[name] = packs.LayerMetadata{
				SHA:  image,
				Data: data,
			}
		}
		metadata.Buildpacks = append(metadata.Buildpacks, packs.BuildpackMetadata{
			Key:    buildpack.ID,
			Layers: layers,
		})
	}

	shPacksBuild, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	image, err = b.addLabelToImage(image, map[string]string{"sh.packs.build": string(shPacksBuild)})
	if err != nil {
		return "", err
	}

	if err := b.Cli.ImageTag(ctx, image, repoName); err != nil {
		return "", err
	}
	return image, nil
}

func (b *BuildFlags) addLabelToImage(image string, labels map[string]string) (string, error) {
	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image:  image,
		Labels: labels,
	}, nil, nil, "")
	if err != nil {
		return "", err
	}
	defer b.Cli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{Force: true})
	res, err := b.Cli.ContainerCommit(ctx, ctr.ID, dockertypes.ContainerCommitOptions{})
	if err != nil {
		return "", err
	}
	return res.ID, nil
}

func dockerBuildExportOLD(group *lifecycle.BuildpackGroup, launchDir, repoName, stackName string) (string, error) {
	var dockerFile string
	dockerFile += "FROM " + stackName + "\n"
	dockerFile += "ADD --chown=packs:packs app /launch/app\n"
	dockerFile += "ADD --chown=packs:packs config /launch/config\n"
	bpLayers := make(map[string][]string)
	numLayers := 0
	needPrevImage := false
	for _, buildpack := range group.Buildpacks {
		dirs, err := filepath.Glob(filepath.Join(launchDir, buildpack.ID, "*.toml"))
		if err != nil {
			return "", err
		}
		bpLayers[buildpack.ID] = dirs
		for _, dir := range dirs {
			if filepath.Base(dir) == "launch.toml" {
				continue
			}
			dir = dir[:len(dir)-5]
			exists := true
			if _, err := os.Stat(dir); err != nil {
				if os.IsNotExist(err) {
					exists = false
				} else {
					return "", err
				}
			}
			dir, err = filepath.Rel(launchDir, dir)
			if err != nil {
				return "", err
			}
			if exists {
				dockerFile += fmt.Sprintf("ADD --chown=packs:packs %s /launch/%s\n", dir, dir)
			} else {
				needPrevImage = true
				dockerFile += fmt.Sprintf("COPY --from=prev --chown=packs:packs /launch/%s /launch/%s\n", dir, dir)
			}
			numLayers++
		}
	}
	if needPrevImage {
		dockerFile = "FROM " + repoName + " AS prev\n\n" + dockerFile
	}
	if err := ioutil.WriteFile(filepath.Join(launchDir, "Dockerfile"), []byte(dockerFile), 0666); err != nil {
		return "", err
	}

	cmd := exec.Command(
		"docker", "build",
		"-t", repoName,
		".",
	)
	cmd.Dir = launchDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	// Layers
	b, err := exec.Command("docker", "inspect", repoName, "-f", "{{json .RootFS.Layers}}").Output()
	if err != nil {
		return "", err
	}
	var imgLayers []string
	if err := json.Unmarshal(b, &imgLayers); err != nil {
		return "", err
	}

	runLayerIDX := len(imgLayers) - numLayers - 3
	metadata := packs.BuildMetadata{
		RunImage: packs.RunImageMetadata{
			Name: stackName,
			SHA:  imgLayers[runLayerIDX],
		},
		App: packs.AppMetadata{
			SHA: imgLayers[runLayerIDX+1],
		},
		Config: packs.ConfigMetadata{
			SHA: imgLayers[runLayerIDX+2],
		},
		Buildpacks: []packs.BuildpackMetadata{},
	}
	bpLayerIDX := runLayerIDX + 2
	for _, buildpack := range group.Buildpacks {
		layers := make(map[string]packs.LayerMetadata)
		for _, tomlFile := range bpLayers[buildpack.ID] {
			name := strings.TrimSuffix(filepath.Base(tomlFile), ".toml")
			if name == "launch" {
				continue
			}
			bpLayerIDX++
			var data interface{}
			if _, err := toml.DecodeFile(tomlFile, &data); err != nil {
				return "", err
			}
			layers[name] = packs.LayerMetadata{
				SHA:  imgLayers[bpLayerIDX],
				Data: data,
			}
		}
		metadata.Buildpacks = append(metadata.Buildpacks, packs.BuildpackMetadata{
			Key:    buildpack.ID,
			Layers: layers,
		})
	}
	b, err = json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	fmt.Printf("\n*** METADATA: %s\n\n", b)

	cmd = exec.Command(
		"docker", "build",
		"-t", repoName,
		"-",
	)
	// lastLayer := strings.TrimPrefix(imgLayers[len(imgLayers)-1], "sha256:")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("FROM %s\nLABEL sh.packs.build '%s'\n", repoName, b))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return "TODO", nil
}
