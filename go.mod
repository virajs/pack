module github.com/buildpack/pack

require (
	github.com/BurntSushi/toml v0.3.0
	github.com/buildpack/forge v0.0.0-20180829150818-bbd9a71e954a0f1f5502170dd6cb3c28a6b3f73f
	github.com/buildpack/lifecycle v0.0.0-20180824184310-a8b0016ee8b0fbf9a6bb926a3e239016fa6b1003
	github.com/buildpack/packs v0.0.0-20180824224129-9ee7ea03c6e5
	github.com/google/go-cmp v0.2.0
	github.com/google/go-containerregistry v0.0.0-20180829201920-2f3e3e1a55fb
	github.com/google/uuid v1.0.0
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/pkg/errors v0.8.0 // indirect
	github.com/sclevine/spec v1.0.0
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.2 // indirect
)

replace (
	github.com/Nvveen/Gotty v0.0.0-20170406111628-a8b993ba6abd => github.com/ijc25/Gotty v0.0.0-20170406111628-a8b993ba6abd
	github.com/docker/docker v0.0.0-20180712004716-371b590ace0d => github.com/docker/engine v0.0.0-20180712004716-371b590ace0d
)
