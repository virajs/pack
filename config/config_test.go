package config_test

import (
	"github.com/buildpack/pack/config"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	spec.Run(t, "config", testConfig, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testConfig(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "pack.config.test.")
		assertNil(t, err)
	})

	it.After(func() {
		err := os.RemoveAll(tmpDir)
		assertNil(t, err)
	})

	when("no config on disk", func() {
		it("writes the defaults to disk", func() {
			subject, err := config.New(tmpDir)
			assertNil(t, err)

			b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
			assertNil(t, err)
			assertContains(t, string(b), `default-stack-id = "io.buildpacks.stacks.bionic"`)
			assertContains(t, string(b), strings.TrimSpace(`
[[stacks]]
  id = "io.buildpacks.stacks.bionic"
  build-images = ["packs/build"]
  run-images = ["packs/run"]
`))

			assertEq(t, len(subject.Stacks), 1)
			assertEq(t, subject.Stacks[0].ID, "io.buildpacks.stacks.bionic")
			assertEq(t, len(subject.Stacks[0].BuildImages), 1)
			assertEq(t, subject.Stacks[0].BuildImages[0], "packs/build")
			assertEq(t, len(subject.Stacks[0].RunImages), 1)
			assertEq(t, subject.Stacks[0].RunImages[0], "packs/run")
			assertEq(t, subject.DefaultStackID, "io.buildpacks.stacks.bionic")
		})
	})

	when("config on disk is missing one of the built-in stacks", func() {
		it.Before(func() {
			w, err := os.Create(filepath.Join(tmpDir, "config.toml"))
			assertNil(t, err)
			defer w.Close()
			w.Write([]byte(`
default-stack-id = "some.user.provided.stack"

[[stacks]]
  id = "some.user.provided.stack"
  build-images = ["some/build"]
  run-images = ["some/run"]
`))
		})

		it("add built-in stack while preserving custom stack and custom default-stack-id", func() {
			subject, err := config.New(tmpDir)
			assertNil(t, err)

			b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
			assertNil(t, err)
			assertContains(t, string(b), `default-stack-id = "some.user.provided.stack"`)
			assertContains(t, string(b), strings.TrimSpace(`
[[stacks]]
  id = "io.buildpacks.stacks.bionic"
  build-images = ["packs/build"]
  run-images = ["packs/run"]
`))
			assertContains(t, string(b), strings.TrimSpace(`
[[stacks]]
  id = "some.user.provided.stack"
  build-images = ["some/build"]
  run-images = ["some/run"]
`))
			assertEq(t, subject.DefaultStackID, "some.user.provided.stack")

			assertEq(t, len(subject.Stacks), 2)
			assertEq(t, subject.Stacks[0].ID, "some.user.provided.stack")
			assertEq(t, len(subject.Stacks[0].BuildImages), 1)
			assertEq(t, subject.Stacks[0].BuildImages[0], "some/build")
			assertEq(t, len(subject.Stacks[0].RunImages), 1)
			assertEq(t, subject.Stacks[0].RunImages[0], "some/run")

			assertEq(t, subject.Stacks[1].ID, "io.buildpacks.stacks.bionic")
			assertEq(t, len(subject.Stacks[1].BuildImages), 1)
			assertEq(t, subject.Stacks[1].BuildImages[0], "packs/build")
			assertEq(t, len(subject.Stacks[1].RunImages), 1)
			assertEq(t, subject.Stacks[1].RunImages[0], "packs/run")
		})
	})

	when("config.toml already has the built-in stack", func() {
		it.Before(func() {
			w, err := os.Create(filepath.Join(tmpDir, "config.toml"))
			assertNil(t, err)
			defer w.Close()
			w.Write([]byte(`
[[stacks]]
  id = "io.buildpacks.stacks.bionic"
  build-images = ["some-other/build"]
  run-images = ["some-other/run", "packs/run"]
`))
		})

		it("does not modify the built-in stack", func() {
			subject, err := config.New(tmpDir)
			assertNil(t, err)

			b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
			assertNil(t, err)
			assertContains(t, string(b), `default-stack-id = "io.buildpacks.stacks.bionic"`)
			assertContains(t, string(b), strings.TrimSpace(`
[[stacks]]
  id = "io.buildpacks.stacks.bionic"
  build-images = ["some-other/build"]
  run-images = ["some-other/run", "packs/run"]
`))

			assertEq(t, len(subject.Stacks), 1)
			assertEq(t, subject.Stacks[0].ID, "io.buildpacks.stacks.bionic")
			assertEq(t, len(subject.Stacks[0].BuildImages), 1)
			assertEq(t, subject.Stacks[0].BuildImages[0], "some-other/build")
			assertEq(t, len(subject.Stacks[0].RunImages), 2)
			assertEq(t, subject.Stacks[0].RunImages[0], "some-other/run")
			assertEq(t, subject.Stacks[0].RunImages[1], "packs/run")
			assertEq(t, subject.DefaultStackID, "io.buildpacks.stacks.bionic")
		})
	})
}

func assertContains(t *testing.T, actual, expected string) {
	t.Helper()
	if !strings.Contains(actual, expected) {
		t.Fatalf("Expected: '%s' inside '%s'", expected, actual)
	}
}

func assertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func assertNotNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual == nil {
		t.Fatal("Expected not nil")
	}
}

func assertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}
