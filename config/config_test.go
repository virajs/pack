package config_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildpack/pack/config"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
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

	when(".BuildConfigFromFlags", func() {
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

			when("path is missing", func() {
				it("creates the directory", func() {
					_, err := config.New(filepath.Join(tmpDir, "a", "b"))
					assertNil(t, err)

					b, err := ioutil.ReadFile(filepath.Join(tmpDir, "a", "b", "config.toml"))
					assertNil(t, err)
					assertContains(t, string(b), `default-stack-id = "io.buildpacks.stacks.bionic"`)
				})
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
	})

	when("Config#Get", func() {
		var subject *config.Config
		it.Before(func() {
			assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
default-stack-id = "my.stack"
[[stacks]]
  id = "stack-1"
[[stacks]]
  id = "my.stack"
[[stacks]]
  id = "stack-3"
`), 0666))
			var err error
			subject, err = config.New(tmpDir)
			assertNil(t, err)
		})
		when("no stack is requested", func() {
			it("returns the default stack", func() {
				stack, err := subject.Get("")
				assertNil(t, err)
				assertEq(t, stack.ID, "my.stack")
			})
		})
		when("a stack known is requested", func() {
			it("returns the stack", func() {
				stack, err := subject.Get("stack-1")
				assertNil(t, err)
				assertEq(t, stack.ID, "stack-1")

				stack, err = subject.Get("stack-3")
				assertNil(t, err)
				assertEq(t, stack.ID, "stack-3")
			})
		})
		when("an unknown stack is requested", func() {
			it("returns an error", func() {
				_, err := subject.Get("stack-4")
				assertNotNil(t, err)
				assertEq(t, err.Error(), `Missing stack: stack with id "stack-4" not found in pack config.toml`)
			})
		})
	})

	when("Config#SetDefaultStack", func() {
		var subject *config.Config
		it.Before(func() {
			assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
default-stack-id = "old.default.stack"
[[stacks]]
  id = "some.stack"
`), 0666))
			var err error
			subject, err = config.New(tmpDir)
			assertNil(t, err)
		})

		when("the stack exists", func() {
			it("sets the default-stack-id", func() {
				err := subject.SetDefaultStack("some.stack")
				assertNil(t, err)
				b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
				assertNil(t, err)
				assertContains(t, string(b), `default-stack-id = "some.stack"`)
			})
		})

		when("the stack doesn't exist", func() {
			it("returns an error and leaves the original default", func() {
				err := subject.SetDefaultStack("some.missing.stack")
				assertError(t, err, `"some.missing.stack" does not exist. Please pass in a valid stack ID.`)
				b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
				assertNil(t, err)
				assertContains(t, string(b), `default-stack-id = "old.default.stack"`)
			})
		})
	})

	when("Config#Add", func() {
		var subject *config.Config
		it.Before(func() {
			assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
default-stack-id = "my.stack"
[[stacks]]
  id = "stack-1"
[[stacks]]
  id = "my.stack"
[[stacks]]
  id = "stack-3"
`), 0666))
			var err error
			subject, err = config.New(tmpDir)
			assertNil(t, err)
		})

		when("stack to be added is new", func() {
			it("adds the stack and writes to file", func() {
				err := subject.Add(config.Stack{
					ID:          "new-stack",
					BuildImages: []string{"neworg/build"},
					RunImages:   []string{"neworg/run"},
				})
				assertNil(t, err)

				stack, err := subject.Get("new-stack")
				assertNil(t, err)
				assertEq(t, stack.ID, "new-stack")
				assertEq(t, stack.BuildImages, []string{"neworg/build"})
				assertEq(t, stack.RunImages, []string{"neworg/run"})

				b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
				assertNil(t, err)
				assertContains(t, string(b), "new-stack")
				assertContains(t, string(b), "neworg/build")
				assertContains(t, string(b), "neworg/run")
			})
		})

		when("stack to be added is already in file", func() {
			it("errors and leaves file unchanged", func() {
				stat, err := os.Stat(filepath.Join(tmpDir, "config.toml"))
				assertNil(t, err)
				origSize := stat.Size()

				err = subject.Add(config.Stack{
					ID:          "my.stack",
					BuildImages: []string{"neworg/build"},
					RunImages:   []string{"neworg/run"},
				})
				assertNotNil(t, err)
				assertEq(t, err.Error(), `stack "my.stack" already exists`)

				stack, err := subject.Get("my.stack")
				assertNil(t, err)
				assertEq(t, stack.BuildImages, []string(nil))

				stat, err = os.Stat(filepath.Join(tmpDir, "config.toml"))
				assertNil(t, err)
				assertEq(t, stat.Size(), origSize)
			})
		})
	})

	when("Config#Update", func() {
		var subject *config.Config
		it.Before(func() {
			assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
default-stack-id = "stack-1"
[[stacks]]
  id = "stack-1"
[[stacks]]
  id = "my.stack"
	build-images = ["packs/build"]
	run-images = ["packs/run"]
[[stacks]]
  id = "stack-3"
`), 0666))
			var err error
			subject, err = config.New(tmpDir)
			assertNil(t, err)
		})

		when("stack to be updated exists", func() {
			it("updates the stack and writes the file", func() {
				err := subject.Update("my.stack", config.Stack{
					BuildImages: []string{"packs/build-2", "fred"},
					RunImages:   []string{"packs/run-2", "jane"},
				})
				assertNil(t, err)

				b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
				assertNil(t, err)
				if !strings.Contains(string(b), "packs/build-2") {
					t.Fatalf(`expected "packs/build-2" to be in config.toml: %s`, b)
				}
				if !strings.Contains(string(b), "packs/run-2") {
					t.Fatalf(`expected "packs/run-2" to be in config.toml: %s`, b)
				}
			})

			it("updates only the fields entered", func() {
				err := subject.Update("my.stack", config.Stack{
					BuildImages: []string{"packs/build-2"},
				})
				assertNil(t, err)
				stack, err := subject.Get("my.stack")
				assertNil(t, err)
				assertEq(t, stack.BuildImages, []string{"packs/build-2"})
				assertEq(t, stack.RunImages, []string{"packs/run"})

				err = subject.Update("my.stack", config.Stack{
					RunImages: []string{"packs/run-3"},
				})
				assertNil(t, err)
				stack, err = subject.Get("my.stack")
				assertNil(t, err)
				assertEq(t, stack.BuildImages, []string{"packs/build-2"})
				assertEq(t, stack.RunImages, []string{"packs/run-3"})
			})
		})

		when("stack to be updated is NOT in file", func() {
			it("errors and leaves file unchanged", func() {
				err := subject.Update("other.stack", config.Stack{
					BuildImages: []string{"packs/build-2"},
					RunImages:   []string{"packs/run-2"},
				})
				assertNotNil(t, err)
				assertEq(t, err.Error(), `Missing stack: stack with id "other.stack" not found in pack config.toml`)
			})
		})
	})

	when("Config#Delete", func() {
		var subject *config.Config
		it.Before(func() {
			assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
default-stack-id = "stack-1"
[[stacks]]
  id = "stack-1"
[[stacks]]
  id = "my.stack"
[[stacks]]
  id = "stack-3"
`), 0666))
			var err error
			subject, err = config.New(tmpDir)
			assertNil(t, err)
		})

		when("stack to be deleted exists", func() {
			it("deletes the stack and writes the file", func() {
				_, err := subject.Get("my.stack")
				assertNil(t, err)

				err = subject.Delete("my.stack")
				assertNil(t, err)

				_, err = subject.Get("my.stack")
				assertNotNil(t, err)

				b, err := ioutil.ReadFile(filepath.Join(tmpDir, "config.toml"))
				assertNil(t, err)
				if strings.Contains(string(b), "my.stack") {
					t.Fatal(`expected "my.stack" to longer be in config.toml`)
				}
			})
		})

		when("stack to be deleted is NOT in file", func() {
			it("errors and leaves file unchanged", func() {
				err := subject.Delete("other.stack")
				assertNotNil(t, err)
				assertEq(t, err.Error(), `"other.stack" does not exist. Please pass in a valid stack ID.`)
			})
		})

		when("stack to be deleted is the default-stack-id", func() {
			it("errors and leaves file unchanged", func() {
				err := subject.Delete("stack-1")
				assertNotNil(t, err)
				assertEq(t, err.Error(), `stack-1 cannot be deleted when it is the default stack. You can change your default stack by running "pack set-default-stack".`)
			})
		})
	})

	when("ImageByRegistry", func() {
		var images []string
		it.Before(func() {
			images = []string{
				"first.com/org/repo",
				"myorg/myrepo",
				"zonal.gcr.io/org/repo",
				"gcr.io/org/repo",
			}
		})
		when("repoName is dockerhub", func() {
			it("returns the dockerhub image", func() {
				name, err := config.ImageByRegistry("index.docker.io", images)
				assertNil(t, err)
				assertEq(t, name, "myorg/myrepo")
			})
		})
		when("registry is gcr.io", func() {
			it("returns the gcr.io image", func() {
				name, err := config.ImageByRegistry("gcr.io", images)
				assertNil(t, err)
				assertEq(t, name, "gcr.io/org/repo")
			})
			when("registry is zonal.gcr.io", func() {
				it("returns the gcr image", func() {
					name, err := config.ImageByRegistry("zonal.gcr.io", images)
					assertNil(t, err)
					assertEq(t, name, "zonal.gcr.io/org/repo")
				})
			})
			when("registry is missingzone.gcr.io", func() {
				it("returns first run image", func() {
					name, err := config.ImageByRegistry("missingzone.gcr.io", images)
					assertNil(t, err)
					assertEq(t, name, "first.com/org/repo")
				})
			})
		})

		when("one of the images is non-parsable", func() {
			it.Before(func() {
				images = []string{"as@ohd@as@op", "gcr.io/myorg/myrepo"}
			})
			it("skips over it", func() {
				name, err := config.ImageByRegistry("gcr.io", images)
				assertNil(t, err)
				assertEq(t, name, "gcr.io/myorg/myrepo")
			})
		})

		when("images is an empty slice", func() {
			it("errors", func() {
				_, err := config.ImageByRegistry("gcr.io", []string{})
				assertNotNil(t, err)
			})
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

func assertError(t *testing.T, actual error, expected string) {
	t.Helper()
	if actual == nil {
		t.Fatalf("Expected an error but got nil")
	}
	if actual.Error() != expected {
		t.Fatalf(`Expected error to equal "%s", got "%s"`, expected, actual.Error())
	}
}
