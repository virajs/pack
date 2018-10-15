package image_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

var registryPort string

func TestRemote(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	log.SetOutput(ioutil.Discard)

	registryContainerName := "test-registry-" + randString(10)
	defer exec.Command("docker", "kill", registryContainerName).Run()
	run(t, exec.Command("docker", "run", "-d", "--rm", "-p", ":5000", "--name", registryContainerName, "registry:2"))
	b, err := exec.Command("docker", "inspect", registryContainerName, "-f", `{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}`).Output()
	assertNil(t, err)
	registryPort = strings.TrimSpace(string(b))

	spec.Run(t, "remote", testRemote, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRemote(t *testing.T, when spec.G, it spec.S) {
	var factory image.Factory
	var buf bytes.Buffer
	var repoName string

	it.Before(func() {
		docker, err := docker.New()
		assertNil(t, err)
		factory = image.Factory{
			Docker: docker,
			Log:    log.New(&buf, "", log.LstdFlags),
			Stdout: &buf,
			FS:     &fs.FS{},
		}
		repoName = "localhost:" + registryPort + "/pack-image-test-" + randString(10)
	})

	when("#NewRemote", func() {
		when("image doesn't exist", func() {
			it.Pend("returns an error", func() {
			})
		})
	})
	when("#Label", func() {
		when("image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(`
					FROM scratch
					LABEL mykey=myvalue other=data
				`)
				assertNil(t, cmd.Run())
				run(t, exec.Command("docker", "push", repoName))
				run(t, exec.Command("docker", "rmi", repoName))
			})

			it("returns the label value", func() {
				img, err := factory.NewRemote(repoName)
				assertNil(t, err)

				label, err := img.Label("mykey")
				assertNil(t, err)
				assertEq(t, label, "myvalue")
			})

			it("returns an empty string for a missing label", func() {
				img, err := factory.NewRemote(repoName)
				assertNil(t, err)

				label, err := img.Label("missing-label")
				assertNil(t, err)
				assertEq(t, label, "")
			})
		})

		when("image NOT exists", func() {
			it("returns an error", func() {
				img, err := factory.NewRemote(repoName)
				assertNil(t, err)

				_, err = img.Label("mykey")
				assertError(t, err, fmt.Sprintf("failed to get label, image '%s' does not exist", repoName))
			})
		})
	})

	when("#Name", func() {
		it("always returns the original name", func() {
			img, _ := factory.NewRemote(repoName)
			assertEq(t, img.Name(), repoName)
		})
	})

	when("#SetLabel", func() {
		when("image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(`
					FROM scratch
					LABEL mykey=myvalue other=data
				`)
				assertNil(t, cmd.Run())
				run(t, exec.Command("docker", "push", repoName))
				run(t, exec.Command("docker", "rmi", repoName))
			})
			it.After(func() {
				exec.Command("docker", "rmi", repoName).Run()
			})

			it("sets label on img object", func() {
				img, _ := factory.NewRemote(repoName)
				assertNil(t, img.SetLabel("mykey", "new-val"))
				label, err := img.Label("mykey")
				assertNil(t, err)
				assertEq(t, label, "new-val")
			})

			it("saves label to docker daemon", func() {
				img, _ := factory.NewRemote(repoName)
				assertNil(t, img.SetLabel("mykey", "new-val"))
				_, err := img.Save()
				assertNil(t, err)

				// Before Pull
				label, err := exec.Command("docker", "inspect", repoName, "-f", `{{.Config.Labels.mykey}}`).Output()
				assertEq(t, strings.TrimSpace(string(label)), "")

				// After Pull
				run(t, exec.Command("docker", "pull", repoName))
				label, err = exec.Command("docker", "inspect", repoName, "-f", `{{.Config.Labels.mykey}}`).Output()
				assertEq(t, strings.TrimSpace(string(label)), "new-val")
			})
		})
	})

	when("#Rebase", func() {
		when("image exists", func() {
			var oldBase, oldTopLayer, newBase string
			it.Before(func() {
				oldBase = "localhost:" + registryPort + "/pack-oldbase-test-" + randString(10)
				oldTopLayer = createImageOnRemote(t, oldBase, `
					FROM busybox
					RUN echo old-base > base.txt
					RUN echo text-old-base > otherfile.txt
				`)

				newBase = "localhost:" + registryPort + "/pack-newbase-test-" + randString(10)
				createImageOnRemote(t, newBase, `
					FROM busybox
					RUN echo new-base > base.txt
					RUN echo text-new-base > otherfile.txt
				`)

				createImageOnRemote(t, repoName, fmt.Sprintf(`
					FROM %s
					RUN echo text-from-image > myimage.txt
					RUN echo text-from-image > myimage2.txt
				`, oldBase))
			})
			it.After(func() {
				exec.Command("docker", "rmi", repoName).Run()
			})

			it.Focus("switches the base", func() {
				// Before
				txt, err := exec.Command("docker", "run", repoName, "cat", "base.txt").Output()
				assertNil(t, err)
				assertEq(t, string(txt), "old-base\n")

				// Run rebase
				img, err := factory.NewRemote(repoName)
				assertNil(t, err)
				newBaseImg, err := factory.NewRemote(newBase)
				assertNil(t, err)
				fmt.Println("BEFORE REBASE")
				err = img.Rebase(oldTopLayer, newBaseImg)
				fmt.Println("AFTER REBASE")
				assertNil(t, err)
				fmt.Println("BEFORE SAVE")
				_, err = img.Save()
				fmt.Println("AFTER SAVE")
				assertNil(t, err)

				// After
				run(t, exec.Command("docker", "pull", repoName))
				txt, err = exec.Command("docker", "run", repoName, "cat", "base.txt").Output()
				assertNil(t, err)
				assertEq(t, string(txt), "new-base\n")
			})
		})
	})
}

func run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to execute command: %v, %s, %s", cmd.Args, err, output)
	}

	return string(output)
}

func createImageOnRemote(t *testing.T, repoName, dockerFile string) string {
	t.Helper()
	defer exec.Command("docker", "rmi", repoName)

	cmd := exec.Command("docker", "build", "-t", repoName, "-")
	cmd.Stdin = strings.NewReader(dockerFile)
	run(t, cmd)

	topLayer, err := exec.Command("docker", "inspect", repoName, "-f", `{{index .RootFS.Layers 2}}`).Output()
	assertNil(t, err)

	run(t, exec.Command("docker", "push", repoName))

	return strings.TrimSpace(string(topLayer))
}
