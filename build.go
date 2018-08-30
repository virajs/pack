package pack

import (
	"archive/tar"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/toml"
	eng "github.com/buildpack/forge/engine"
	engdocker "github.com/buildpack/forge/engine/docker"
	"github.com/buildpack/lifecycle"
	"github.com/google/uuid"
)

func Build(appDir, detectImage, repoName string, publish bool) error {
	return (&BuildFlags{
		AppDir:      appDir,
		DetectImage: detectImage,
		RepoName:    repoName,
		Publish:     publish,
	}).Run()
}

type BuildFlags struct {
	AppDir          string
	DetectImage     string
	RepoName        string
	Publish         bool
	Engine          eng.Engine
	launchVolume    string
	workspaceVolume string
	cacheVolume     string
}

func (b *BuildFlags) Init() error {
	var err error
	b.AppDir, err = filepath.Abs(b.AppDir)
	if err != nil {
		return err
	}

	uid := uuid.New().String()
	b.launchVolume = fmt.Sprintf("pack-launch-%x", uid)
	b.workspaceVolume = fmt.Sprintf("pack-workspace-%x", uid)
	b.cacheVolume = fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(b.AppDir)))

	b.Engine, err = engdocker.New(&eng.EngineConfig{})
	return err
}

func (b *BuildFlags) Run() error {
	if err := b.Init(); err != nil {
		return err
	}
	defer exec.Command("docker", "volume", "rm", "-f", b.launchVolume).Run()
	defer exec.Command("docker", "volume", "rm", "-f", b.workspaceVolume).Run()

	waitFor(b.Engine.NewImage().Pull(b.DetectImage))
	fmt.Println("*** COPY APP TO VOLUME:")
	if err := b.UploadDirToVolume(b.AppDir, "/launch/app"); err != nil {
		return err
	}

	fmt.Println("*** DETECTING:")
	group, err := b.Detect()
	if err != nil {
		return err
	}

	fmt.Println("*** ANALYZING: Reading information from previous image for possible re-use")
	if err := b.Analyze(group); err != nil {
		return err
	}

	fmt.Println("*** BUILDING:")
	waitFor(b.Engine.NewImage().Pull(group.BuildImage))
	if err := b.Build(group); err != nil {
		return err
	}

	if !b.Publish {
		fmt.Println("*** PULLING RUN IMAGE LOCALLY:")
		waitFor(b.Engine.NewImage().Pull(group.RunImage))
	}

	fmt.Println("*** EXPORTING:")
	imgSHA, err := b.Export(group)
	if err != nil {
		return err
	}

	if b.Publish {
		fmt.Printf("\n*** Image: %s@%s\n", b.RepoName, imgSHA)
	}

	return nil
}

func (b *BuildFlags) UploadDirToVolume(srcDir, destDir string) error {
	cont, err := b.Engine.NewContainer(&eng.ContainerConfig{
		Name:  "pack-upload",
		Image: b.DetectImage,
		Binds: []string{
			b.launchVolume + ":/launch",
			b.workspaceVolume + ":/workspace",
		},
		// TODO below is very problematic
		Entrypoint: []string{},
		Cmd:        []string{"chown", "-R", "packs", destDir},
		User:       "root",
	})
	if err != nil {
		return err
	}
	defer cont.Close()
	tr, err := createTarReader(srcDir, destDir)
	if err != nil {
		return err
	}
	if err := cont.UploadTarTo(tr, "/"); err != nil {
		return err
	}
	if exitStatus, err := cont.Start("", os.Stdout, nil); err != nil {
		return err
	} else if exitStatus != 0 {
		return fmt.Errorf("upload failed with: %d", exitStatus)
	}
	return nil
}

func (b *BuildFlags) ExportVolume(path string) (string, func(), error) {
	cont, err := b.Engine.NewContainer(&eng.ContainerConfig{
		Name:  "pack-export",
		Image: b.DetectImage,
		Binds: []string{
			b.launchVolume + ":/launch",
		},
	})
	if err != nil {
		return "", func() {}, err
	}
	defer cont.Close()

	tmpDir, err := ioutil.TempDir("", "pack.build.")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	fmt.Println("*   StreamTarFrom:", path)
	r, err := cont.StreamTarFrom(path)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}

	fmt.Println("*   UntarReader:", tmpDir)
	if err := untarReader(r, tmpDir); err != nil {
		fmt.Println("FAILED TO UNTAR READER")
		cleanup()
		return "", func() {}, err
	}

	fmt.Println("*   ExportVolume done")
	return tmpDir, cleanup, nil
}

func (b *BuildFlags) Detect() (lifecycle.BuildpackGroup, error) {
	detectCont, err := b.Engine.NewContainer(&eng.ContainerConfig{
		Name:  "pack-detect",
		Image: b.DetectImage,
		Binds: []string{
			b.launchVolume + ":/launch",
			b.workspaceVolume + ":/workspace",
		},
	})
	if err != nil {
		return lifecycle.BuildpackGroup{}, err
	}
	defer detectCont.Close()

	if exitStatus, err := detectCont.Start("", os.Stdout, nil); err != nil {
		return lifecycle.BuildpackGroup{}, err
	} else if exitStatus != 0 {
		return lifecycle.BuildpackGroup{}, fmt.Errorf("detect failed with: %d", exitStatus)
	}

	return b.GroupToml(detectCont)
}

func (b *BuildFlags) Analyze(group lifecycle.BuildpackGroup) error {
	analyzeTmpDir, err := ioutil.TempDir("", "pack.build.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(analyzeTmpDir)
	if err := analyzer(group, analyzeTmpDir, b.RepoName, !b.Publish); err != nil {
		return err
	}
	return b.UploadDirToVolume(analyzeTmpDir, "/launch")
}

func (b *BuildFlags) Build(group lifecycle.BuildpackGroup) error {
	buildCont, err := b.Engine.NewContainer(&eng.ContainerConfig{
		Name:  "pack-build",
		Image: group.BuildImage,
		Binds: []string{
			b.launchVolume + ":/launch",
			b.workspaceVolume + ":/workspace",
			b.cacheVolume + ":/cache",
		},
	})
	if err != nil {
		return err
	}
	defer buildCont.Close()
	if exitStatus, err := buildCont.Start("", os.Stdout, nil); err != nil {
		return err
	} else if exitStatus != 0 {
		return fmt.Errorf("build failed with: %d", exitStatus)
	}
	return nil
}

func (b *BuildFlags) GroupToml(container eng.Container) (lifecycle.BuildpackGroup, error) {
	r, err := container.StreamFileFrom("/workspace/group.toml")
	if err != nil {
		return lifecycle.BuildpackGroup{}, err
	}

	txt, err := ioutil.ReadAll(r)
	if err != nil {
		return lifecycle.BuildpackGroup{}, err
	}

	var group lifecycle.BuildpackGroup
	if _, err := toml.Decode(string(txt), &group); err != nil {
		return lifecycle.BuildpackGroup{}, err
	}

	return group, nil
}

func (b *BuildFlags) Export(group lifecycle.BuildpackGroup) (string, error) {
	localLaunchDir, cleanup, err := b.ExportVolume("/launch")
	if err != nil {
		return "", err
	}
	defer cleanup()

	imgSHA, err := export(group, localLaunchDir, b.RepoName, group.RunImage, !b.Publish, !b.Publish)
	if err != nil {
		return "", err
	}
	return imgSHA, nil
}

// TODO share between here and create.go (and exporter).
func createTarReader(fsDir, tarDir string) (io.ReadCloser, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer tw.Close()

	err := filepath.Walk(fsDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode().IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(fsDir, file)
		if err != nil {
			return err
		}

		var header *tar.Header
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(file)
			if err != nil {
				return err
			}
			header, err = tar.FileInfoHeader(fi, target)
			if err != nil {
				return err
			}
		} else {
			header, err = tar.FileInfoHeader(fi, fi.Name())
			if err != nil {
				return err
			}
		}
		header.Name = filepath.Join(tarDir, relPath)
		fmt.Println("    ", header.Name)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ioutil.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

func untarReader(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		f, err := tr.Next()
		if err == io.EOF {
			fmt.Println("    EOF")
			break
		}
		if err != nil {
			fmt.Println("    tar error:", err)
			return fmt.Errorf("tar error: %v", err)
		}
		abs := filepath.Join(dir, f.Name)

		fi := f.FileInfo()
		mode := fi.Mode()
		switch {
		case mode.IsRegular():
			if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
				return err
			}
			w, err := os.OpenFile(abs, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode.Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(w, tr); err != nil {
				return fmt.Errorf("error writing to %s: %v", abs, err)
			}
			if err := w.Close(); err != nil {
				return err
			}
		case mode.IsDir():
			if err := os.MkdirAll(abs, 0755); err != nil {
				return err
			}
		case fi.Mode()&os.ModeSymlink != 0:
			if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
				return err
			}
			if err := os.Symlink(f.Linkname, abs); err != nil {
				return err
			}
		default:
			fmt.Println("tar unsupported:", f.Name, mode)
			return fmt.Errorf("tar file entry %s contained unsupported file type %v", f.Name, mode)
		}
	}
	fmt.Println("    TAR DONE")
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
