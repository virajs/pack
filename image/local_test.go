package image_test

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/image"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestLocal(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "local", testLocal, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLocal(t *testing.T, when spec.G, it spec.S) {
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
		}
		repoName = "pack-image-test-" + randString(10)
	})

	when("#NewLocal", func() {
		when("pull is false and the image doesn't exist", func() {
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
			})
			it("returns the label value", func() {
				img, err := factory.NewLocal(repoName, false)
				assertNil(t, err)

				label, err := img.Label("mykey")
				assertNil(t, err)
				assertEq(t, label, "myvalue")
			})

			it("returns an empty string for a missing label", func() {
				img, err := factory.NewLocal(repoName, false)
				assertNil(t, err)

				label, err := img.Label("missing-label")
				assertNil(t, err)
				assertEq(t, label, "")
			})
		})

		when("image NOT exists", func() {
			it("returns an error", func() {
				img, err := factory.NewLocal(repoName, false)
				assertNil(t, err)

				_, err = img.Label("mykey")
				assertError(t, err, fmt.Sprintf("failed to get label, image '%s' does not exist", repoName))
			})
		})
	})

	when("#Name", func() {
		it("always returns the original name", func() {
			img, _ := factory.NewLocal(repoName, false)
			assertEq(t, img.Name(), repoName)
		})
	})

	when("#SetLabel", func() {
		it("sets label on img object", func() {
			img, _ := factory.NewLocal(repoName, false)
			assertNil(t, img.SetLabel("mykey", "new-val"))
			label, err := img.Label("mykey")
			assertNil(t, err)
			assertEq(t, label, "new-val")
		})

		it("saves label to docker daemon", func() {
			img, _ := factory.NewLocal(repoName, false)
			assertNil(t, img.SetLabel("mykey", "new-val"))
			_, err := img.Save()
			assertNil(t, err)

			label, err := exec.Command("docker", "inspect", repoName, "-f", `{{.Config.Labels}}`).Output()
			assertEq(t, label, "new-val")
		})
	})
}

func assertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

// Assert deep equality (and provide useful difference as a test failure)
func assertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}

func assertError(t *testing.T, actual error, expected string) {
	t.Helper()
	if actual == nil {
		t.Fatalf("Expected an error but got nil")
	}
	if actual.Error() != expected {
		t.Fatalf(`Expected error to equal "%s", got "%s"`, expected, actual.Error())
	}
}
