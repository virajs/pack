package pack

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

func Build(appDir, buildImage, runImage, repoName string, publish bool) (err error) {
	buildFlags := &BuildFlags{
		AppDir:     appDir,
		BuildImage: buildImage,
		RunImage:   runImage,
		RepoName:   repoName,
		Publish:    publish,
	}
	buildFlags.Cli, err = dockercli.NewEnvClient()
	if err != nil {
		return err
	}
	return buildFlags.Run()
}

type BuildFlags struct {
	AppDir     string
	BuildImage string
	RunImage   string
	RepoName   string
	Publish    bool
	Cli        *dockercli.Client
}

func (b *BuildFlags) Run() error {
	var err error
	b.AppDir, err = filepath.Abs(b.AppDir)
	if err != nil {
		return err
	}

	uid := uuid.New().String()
	launchVolume := fmt.Sprintf("pack-launch-%x", uid)
	workspaceVolume := fmt.Sprintf("pack-workspace-%x", uid)
	cacheVolume := fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(b.AppDir)))
	defer b.Cli.VolumeRemove(context.Background(), launchVolume, true)
	defer b.Cli.VolumeRemove(context.Background(), workspaceVolume, true)

	fmt.Println("*** COPY APP TO VOLUME:")
	if err := copyToVolume(b.BuildImage, launchVolume, b.AppDir, "app"); err != nil {
		return err
	}

	fmt.Println("*** DETECTING:")
	group, err := b.Detect(uid, launchVolume, workspaceVolume)
	if err != nil {
		return err
	}

	fmt.Println("*** ANALYZING: Reading information from previous image for possible re-use")
	if err := b.Analyze(uid, launchVolume, workspaceVolume); err != nil {
		return err
	}

	fmt.Println("*** BUILDING:")
	if err := b.Build(uid, launchVolume, workspaceVolume, cacheVolume); err != nil {
		return err
	}

	if !b.Publish {
		fmt.Println("*** PULLING RUN IMAGE LOCALLY:")
		rc, err := b.Cli.ImagePull(context.Background(), b.RunImage, dockertypes.ImagePullOptions{})
		if err != nil {
			return err
		}
		io.Copy(ioutil.Discard, rc)
		rc.Close()
	}

	fmt.Println("*** EXPORTING:")
	if err := b.Export(uid, group, launchVolume, workspaceVolume, cacheVolume); err != nil {
		return err
	}

	return nil
}

func (b *BuildFlags) Detect(uid, launchVolume, workspaceVolume string) (*lifecycle.BuildpackGroup, error) {
	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image:      b.BuildImage,
		Entrypoint: []string{"/packs/detector"},
	}, &container.HostConfig{
		Binds: []string{
			launchVolume + ":/launch",
			workspaceVolume + ":/workspace",
		},
	}, nil, "pack-detect-"+uid)
	if err != nil {
		return nil, err
	}
	defer b.Cli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{Force: true})

	if err := b.runContainer(ctx, ctr.ID, os.Stdout, os.Stderr); err != nil {
		return nil, err
	}

	return b.groupToml(ctr.ID)
}

func (b *BuildFlags) Analyze(uid, launchVolume, workspaceVolume string) (err error) {
	var shPacksBuild string
	ctx := context.Background()
	if !b.Publish {
		i, _, err := b.Cli.ImageInspectWithRaw(ctx, b.RepoName)
		if dockercli.IsErrNotFound(err) {
			fmt.Println("    No previous image found")
			return nil
		}
		if err != nil {
			return err
		}
		shPacksBuild = i.Config.Labels["sh.packs.build"]
		if shPacksBuild == "" {
			fmt.Println("    Previous image is missing label 'sh.packs.build'")
			return nil
		}
	}

	cfg := &container.Config{
		Image: "dgodd/packsv3:analyze",
	}
	hcfg := &container.HostConfig{
		Binds: []string{
			launchVolume + ":/launch",
			workspaceVolume + ":/workspace",
		},
	}

	if b.Publish {
		cfg.Env = []string{"PACK_USE_HELPERS=true"}
		cfg.Cmd = []string{b.RepoName}
		hcfg.Binds = append(hcfg.Binds, filepath.Join(os.Getenv("HOME"), ".docker")+":/home/packs/.docker:ro")
	} else {
		cfg.Cmd = []string{"-metadata", "/tmp/metadata.json", b.RepoName}
		cfg.OpenStdin = true
		cfg.StdinOnce = true
	}

	fmt.Println("    pull image")
	rc, err := b.Cli.ImagePull(ctx, cfg.Image, dockertypes.ImagePullOptions{})
	if err != nil {
		return err
	}
	io.Copy(ioutil.Discard, rc)
	rc.Close()

	fmt.Println("    create container")
	ctr, err := b.Cli.ContainerCreate(ctx, cfg, hcfg, nil, "pack-analyze-"+uid)
	if err != nil {
		return err
	}
	defer b.Cli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{Force: true})

	tr, err := singleFileTar("metadata.json", shPacksBuild)
	if err != nil {
		return err
	}
	if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/tmp", tr, dockertypes.CopyToContainerOptions{}); err != nil {
		return err
	}

	return b.runContainer(ctx, ctr.ID, os.Stdout, os.Stderr)
}

func (b *BuildFlags) Build(uid, launchVolume, workspaceVolume, cacheVolume string) error {
	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image:      b.BuildImage,
		Entrypoint: []string{"/packs/builder"},
	}, &container.HostConfig{
		Binds: []string{
			launchVolume + ":/launch",
			workspaceVolume + ":/workspace",
			cacheVolume + ":/cache",
		},
	}, nil, "pack-build-"+uid)
	if err != nil {
		return err
	}
	defer b.Cli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{Force: true})

	return b.runContainer(ctx, ctr.ID, os.Stdout, os.Stderr)
}

func (b *BuildFlags) Export(uid string, group *lifecycle.BuildpackGroup, launchVolume, workspaceVolume, cacheVolume string) error {
	if b.Publish {
		ctx := context.Background()
		image := "dgodd/packsv3:export"

		rc, err := b.Cli.ImagePull(ctx, image, dockertypes.ImagePullOptions{})
		if err != nil {
			return err
		}
		io.Copy(ioutil.Discard, rc)
		rc.Close()

		ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
			Image: image,
			Env:   []string{"PACK_USE_HELPERS=true", "PACK_RUN_IMAGE=" + b.RunImage},
			Cmd:   []string{b.RepoName},
		}, &container.HostConfig{
			Binds: []string{
				launchVolume + ":/launch",       // TODO I think this can be READONLY
				workspaceVolume + ":/workspace", // TODO I think this can be deleted
				filepath.Join(os.Getenv("HOME"), ".docker") + ":/home/packs/.docker:ro",
			},
		}, nil, "pack-export-"+uid)
		if err != nil {
			return err
		}
		defer b.Cli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{Force: true})

		return b.runContainer(ctx, ctr.ID, os.Stdout, os.Stderr)
	}

	fullStart := time.Now()
	start := time.Now()
	launchData, err := b.launchData(b.BuildImage, launchVolume)
	if err != nil {
		return err
	}
	fmt.Println(launchData)
	fmt.Printf("    extract launch data to host: %s\n", time.Since(start))
	start = time.Now()

	// _, err = dockerBuildExport(group, localLaunchDir, b.RepoName, b.RunImage)
	_, err = b.dockerBuildExport(group, launchVolume, launchData, b.RepoName, b.RunImage)
	if err != nil {
		return err
	}
	fmt.Printf("    create image: %s (%s)\n", time.Since(start), time.Since(fullStart))
	return nil
}

type LaunchData struct {
	Dirs  map[string]bool        `json:"dirs"`
	Files map[string][]string    `json:"files"`
	Data  map[string]interface{} `json:"data"`
}

func (b *BuildFlags) launchData(image, launchVolume string) (LaunchData, error) {
	fmt.Println("DG: b.launchData: 1")
	script := `
set -eo pipefail
JSON='{}'
for dir in $DIRS; do
  for file in $(ls "$dir"); do
    if [[ -d "$dir/$file" ]]; then
      JSON=$(echo $JSON | jq --argjson file "\"$dir/$file\"" '.dirs[$file] = true')
    fi
  done
  for file in $(ls "$dir"/*.toml); do
    if [[ $(basename $file) != "launch.toml" ]]; then
      data=$(yj -t < "$file")
      JSON=$(echo $JSON | jq --argjson dir "\"$dir\"" --argjson file "\"$(basename $file)\"" '.files[$dir] += [$file]')
      JSON=$(echo $JSON | jq --argjson file "\"$file\"" --argjson data "$data" '.data[$file] = $data')
    fi
  done
done
echo $JSON
`
	tr, err := singleFileTar("launchdata.sh", script)
	if err != nil {
		return LaunchData{}, err
	}

	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image:      image,
		Entrypoint: []string{},
		Cmd:        []string{"bash", "/tmp/launchdata.sh"},
		Env:        []string{"DIRS=sh.packs.samples.buildpack.nodejs"}, // FIXME get from group.Buildpacks
		WorkingDir: "/launch",
	}, &container.HostConfig{
		Binds: []string{
			launchVolume + ":/launch",
		},
	}, nil, "dgodd")
	if err != nil {
		return LaunchData{}, errors.Wrap(err, "launchData: create container")
	}
	defer b.Cli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{Force: true})

	if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/tmp", tr, dockertypes.CopyToContainerOptions{}); err != nil {
		return LaunchData{}, errors.Wrap(err, "launchData: copy to container")
	}

	var buf, buferr bytes.Buffer
	if err := b.runContainer(ctx, ctr.ID, &buf, &buferr); err != nil {
		fmt.Println("RUN CONTAINER OUT:", buf.String())
		return LaunchData{}, errors.Wrap(err, "launchData: run container")
	}

	var data LaunchData
	bufString := strings.TrimSpace(buf.String())
	if err := json.Unmarshal([]byte(bufString), &data); err != nil {
		fmt.Printf("DATA: |%s|\n", bufString)
		fmt.Printf("STDERR: |%s|\n", buferr.String())
		return LaunchData{}, errors.Wrap(err, "launchData: parsing json")
	}

	return data, nil
}

func copyToVolume(image, volName, srcDir, destDir string) error {
	containerName := uuid.New().String()
	if output, err := exec.Command("docker", "container", "create", "--user", "0", "--name", containerName, "--entrypoint", "", "-v", volName+":/launch", image, "chown", "-R", "packs:packs", "/launch").CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return err
	}
	defer exec.Command("docker", "rm", containerName).Run()
	if output, err := exec.Command("docker", "cp", srcDir+"/.", containerName+":"+filepath.Join("/launch", destDir)).CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return err
	}

	if output, err := exec.Command("docker", "start", containerName).CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return err
	}
	return nil
}

func (b *BuildFlags) groupToml(ctrID string) (*lifecycle.BuildpackGroup, error) {
	ctx := context.Background()
	rc, _, err := b.Cli.CopyFromContainer(ctx, ctrID, "/workspace/group.toml")
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	tr := tar.NewReader(rc)
	if _, err = tr.Next(); err != nil {
		return nil, err
	}
	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeReader(tr, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

func (b *BuildFlags) runContainer(ctx context.Context, id string, stdout io.Writer, stderr io.Writer) error {
	if err := b.Cli.ContainerStart(ctx, id, dockertypes.ContainerStartOptions{}); err != nil {
		return err
	}
	stdout2, err := b.Cli.ContainerLogs(ctx, id, dockertypes.ContainerLogsOptions{
		ShowStdout: true,
		Follow:     true,
	})
	if err != nil {
		return err
	}
	go func() {
		for {
			header := make([]byte, 8)
			stdout2.Read(header)
			if _, err := io.CopyN(stdout, stdout2, int64(binary.BigEndian.Uint32(header[4:]))); err != nil {
				break
			}
		}
	}()
	stderr2, err := b.Cli.ContainerLogs(ctx, id, dockertypes.ContainerLogsOptions{
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return err
	}
	go func() {
		for {
			header := make([]byte, 8)
			stderr2.Read(header)
			if _, err := io.CopyN(stderr, stderr2, int64(binary.BigEndian.Uint32(header[4:]))); err != nil {
				break
			}
		}
	}()

	waitC, errC := b.Cli.ContainerWait(ctx, id, "")
	select {
	case w := <-waitC:
		if w.StatusCode != 0 {
			return fmt.Errorf("container run: non zero exit: %d: %s", w.StatusCode, w.Error)
		}
		return nil
	case err := <-errC:
		return err
	}
}
