package pack

import (
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
)

func dockerBuildExport(group lifecycle.BuildpackGroup, launchDir, repoName, stackName string) (string, error) {
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
