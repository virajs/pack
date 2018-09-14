package pack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/img"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

type CreateBuilderFlags struct {
	RepoName        string
	BuilderTomlPath string
}

type Buildpack struct {
     ID string
     URI string
}

type Builder struct {
	Buildpacks []Buildpack
	Order lifecycle.BuildpackOrder
}

type BuilderFactory struct {
	DefaultStack Stack
}

func (f *BuilderFactory) Create(flags CreateBuilderFlags) error {
	builderStore, err := repoStore(flags.RepoName, true)
	if err != nil {
		return err
	}

	baseImage, err := readImage(f.DefaultStack.BuildImage, true)
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir("", "buildpack")
	if err != nil {
		return err
	}
	defer os.Remove(tmpDir)

	buildpackDir, err := f.buildpackDir(tmpDir)
	if err != nil {
		return err
	}
	if err := createTarFile(filepath.Join(tmpDir, "buildpacks.tar"), buildpackDir, "/buildpacks"); err != nil {
		return err
	}
	builderImage, _, err := img.Append(baseImage, filepath.Join(tmpDir, "buildpacks.tar"))
	if err != nil {
		return err
	}


	return  builderStore.Write(builderImage)
}

func (f * BuilderFactory) buildpackDir(dest string) (string, error){
	buildpackDir := filepath.Join(dest, "buildpack")
	err := os.Mkdir(buildpackDir, 0755)
	if err != nil {
		return "", err
	}
	orderFile, err := os.Create(filepath.Join(buildpackDir, "order.toml"))
	if err != nil {
		return "", err
	}
	if err := orderFile.Close(); err != nil {
		return "", err
	}
	return buildpackDir, nil
}


// TODO share between here and exporter.
func createTarFile(tarFile, fsDir, tarDir string) error {
	fh, err := os.Create(tarFile)
	if err != nil {
		return fmt.Errorf("create file for tar: %s", err)
	}
	defer fh.Close()
	gzw := gzip.NewWriter(fh)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	return filepath.Walk(fsDir, func(file string, fi os.FileInfo, err error) error {
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
}
