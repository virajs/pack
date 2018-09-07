package pack

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
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

func simpleExport(group lifecycle.BuildpackGroup, launchDir, repoName, stackName string) (string, error) {
	metadata := packs.BuildMetadata{
		App:        packs.AppMetadata{},
		Buildpacks: []packs.BuildpackMetadata{},
		RunImage:   packs.RunImageMetadata{},
	}

	httpc := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
	}

	fmt.Println("STACK:", stackName)
	res, err := httpc.Get("http://unix/images/" + stackName + "/get")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("expected 200: actual: %d", res.StatusCode)
	}

	r, w := io.Pipe()
	// tarball := tar.NewWriter(bufio.NewWriterSize(w, 1048576))
	tarball := tar.NewWriter(w)

	var parentLayerID string
	go func() {
		tarReader := tar.NewReader(res.Body)
		runImageJson := make(map[string][]byte)
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				panic(err)
			}

			if header.Name == "repositories" {
				fmt.Println("REPOSITORIES")
				out := make(map[string]map[string]string)
				json.NewDecoder(tarReader).Decode(&out)
				// io.Copy(os.Stdout, tarReader)
				// fmt.Printf("OUT: %#v\n", out)
				for _, v1 := range out {
					for _, v2 := range v1 {
						parentLayerID = v2
					}
				}
				if parentLayerID == "" {
					panic("could not determine top of stack")
				}
				metadata.RunImage.SHA = parentLayerID
			}

			if !(strings.HasSuffix(header.Name, "/VERSION") || strings.HasSuffix(header.Name, "/json") || strings.HasSuffix(header.Name, "/layer.tar")) {
				continue
			}

			// fmt.Println(header.Name, header.FileInfo())
			fmt.Println("From stack:", header.Name)

			if err := tarball.WriteHeader(header); err != nil {
				panic(err)
			}

			if strings.HasSuffix(header.Name, "/json") {
				buf, err := ioutil.ReadAll(tarReader)
				if err != nil {
					panic(err)
				}
				m := strings.Split(header.Name, "/")
				runImageJson[m[0]] = buf
				if _, err := tarball.Write(buf); err != nil {
					panic(err)
				}
			} else {
				if _, err = io.Copy(tarball, tarReader); err != nil {
					panic(err)
				}
			}
		}

		var imgConfig map[string]interface{}
		if err := json.Unmarshal(runImageJson[parentLayerID], &imgConfig); err != nil {
			panic(err)
		}

		layerDirs := []string{"app", "config"}
		for _, buildpack := range group.Buildpacks {
			dirs, err := filepath.Glob(filepath.Join(launchDir, buildpack.ID, "*.toml"))
			if err != nil {
				panic(err)
			}
			for _, dir := range dirs {
				dir = dir[:len(dir)-5]
				fmt.Println("DIR:", dir)
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					fmt.Println("DIR NOT EXIST:", dir)
					continue
				} else if err != nil {
					panic(err)
				}
				dir, err = filepath.Rel(launchDir, dir)
				if err != nil {
					panic(err)
				}
				layerDirs = append(layerDirs, dir)
			}
		}

		fmt.Println("LAYER DIRS:", layerDirs)

		bpMeta := make(map[string]packs.BuildpackMetadata)

		for _, name := range layerDirs {
			start := time.Now()

			b, err := tarDir(filepath.Join(launchDir, name), "launch/"+name)
			if err != nil {
				panic(err)
			}

			layerID := fmt.Sprintf("%x", sha256.Sum256(b))
			addFileToTar(tarball, layerID+"/VERSION", []byte("1.0"))
			layerData := map[string]interface{}{
				"id":     layerID,
				"parent": parentLayerID,
				"os":     "linux",
			}
			layerDataJSON, err := json.Marshal(layerData)
			if err != nil {
				panic(nil)
			}
			addFileToTar(tarball, layerID+"/json", layerDataJSON)
			addFileToTar(tarball, layerID+"/layer.tar", b)
			parentLayerID = layerID

			switch name {
			case "app":
				metadata.App.SHA = layerID
			case "config":
				metadata.Config.SHA = layerID
			default:
				m := strings.SplitN(name, "/", 2)
				obj := bpMeta[m[0]]
				obj.Key = m[0]
				if obj.Layers == nil {
					obj.Layers = make(map[string]packs.LayerMetadata)
				}
				var tomlData interface{}
				_, err := toml.DecodeFile(filepath.Join(launchDir, name+".toml"), &tomlData)
				if err != nil {
					panic(nil)
				}
				obj.Layers[m[1]] = packs.LayerMetadata{SHA: layerID, Data: tomlData}
				bpMeta[m[0]] = obj
			}

			fmt.Printf("Full add tar for (%s): %s (%d)\n", time.Since(start), name, len(b))
		}

		for _, bp := range bpMeta {
			metadata.Buildpacks = append(metadata.Buildpacks, bp)
		}

		// Create layer for metadata label
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			panic(err)
		}
		layerID := fmt.Sprintf("%x", sha256.Sum256(metadataJSON))
		addFileToTar(tarball, layerID+"/VERSION", []byte("1.0"))
		imgConfig["id"] = layerID
		imgConfig["parent"] = parentLayerID
		imgConfig["created"] = time.Now()
		if h, ok := imgConfig["container_config"].(map[string]interface{}); ok {
			// TODO: Keep other labels
			h["Labels"] = map[string]interface{}{
				"packs.sh": string(metadataJSON),
			}
		} else {
			panic("could not set labels on container config")
		}
		if h, ok := imgConfig["config"].(map[string]interface{}); ok {
			// TODO: Keep other labels
			h["Labels"] = map[string]interface{}{
				"packs.sh": string(metadataJSON),
			}
		} else {
			panic("could not set labels on config")
		}
		layerDataJSON, err := json.Marshal(imgConfig)
		if err != nil {
			panic(nil)
		}
		fmt.Println(layerID, " => ", string(layerDataJSON))
		addFileToTar(tarball, layerID+"/json", layerDataJSON)
		var emptyTar bytes.Buffer
		tar.NewWriter(&emptyTar).Close()
		addFileToTar(tarball, layerID+"/layer.tar", emptyTar.Bytes())
		parentLayerID = layerID

		// TODO repoName may have a tag, need to fix that
		if err := addFileToTar(tarball, "repositories", []byte(fmt.Sprintf(`{"%s":{"latest":"%s"}}`, repoName, parentLayerID))); err != nil {
			panic(err)
		}

		fmt.Println("FINAL PARENT ID:", parentLayerID)

		tarball.Close()
		w.Close()

		fmt.Println("**** closed tarball ****")
	}()

	debugTarFile, _ := os.Create("/tmp/debug_tar_file.tar")
	r2 := io.TeeReader(r, debugTarFile)
	defer debugTarFile.Close()

	res, err = httpc.Post("http://unix/images/load?quiet=true", "application/tar", r2)
	r.Close()
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("expected 200: actual: %d", res.StatusCode)
	}
	fmt.Printf("\n\nPOST: %#v\n\n", res)
	io.Copy(os.Stdout, res.Body)
	fmt.Println("")
	fmt.Println("FINAL PARENT ID:", parentLayerID)

	return parentLayerID, nil
}

func simpleExportOld(group lifecycle.BuildpackGroup, launchDir, repoName, stackName string) (string, error) {
	cmd := exec.Command("docker", "build", "--force-rm", "-t", repoName, "-f", "-", launchDir)
	cmd.Dir = launchDir
	cmd.Stdin = strings.NewReader(fmt.Sprintf("FROM %s\nCOPY . /launch\n", stackName))
	if txt, err := cmd.CombinedOutput(); err != nil {
		fmt.Println(string(txt))
		return "", err
	}
	return "TODO", nil
}

func addFileToTar(w *tar.Writer, path string, contents []byte) error {
	if err := w.WriteHeader(&tar.Header{Name: path, Size: int64(len(contents))}); err != nil {
		return err
	}
	_, err := w.Write([]byte(contents))
	return err
}

func tarDir(srcDir, destDir string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		destPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		destPath = filepath.Join(destDir, destPath)
		if info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			fmt.Println("SYMLINK:", destPath)
			// TODO handle correctly
			return nil
		}

		if err := tw.WriteHeader(&tar.Header{
			Name:    destPath,
			Size:    info.Size(),
			Mode:    int64(info.Mode()),
			ModTime: info.ModTime(),
		}); err != nil {
			return err
		}

		fh, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fh.Close()
		_, err = io.Copy(tw, fh)
		return err
	})
	tw.Close()
	return buf.Bytes(), err
}
