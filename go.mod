module github.com/buildpack/pack

require (
	github.com/BurntSushi/toml v0.3.0
	github.com/buildpack/lifecycle v0.0.0-20180824000627-2f640c42a336dbb896d0935c8adffdb72b1c558d
	github.com/buildpack/packs v0.0.0-20180824001031-aa30a412923763df37e83f14a6e4e0fe07e11f25
	github.com/docker/docker v0.0.0-20180531152204-71cd53e4a197
	github.com/golang/mock v1.1.1 // indirect
	github.com/google/go-cmp v0.2.0
	github.com/google/go-containerregistry v0.0.0-20180731221751-697ee0b3d46e
	github.com/google/uuid v0.0.0-20171129191014-dec09d789f3d
	github.com/moby/moby v0.0.0-20180531152204-71cd53e4a197
	github.com/sclevine/spec v0.0.0-20180404042546-a925ac4bfbc9
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.2 // indirect
)

replace github.com/google/go-containerregistry v0.0.0-20180731221751-697ee0b3d46e => github.com/dgodd/go-containerregistry v0.0.0-20180731221751-611aad063148a69435dccd3cf8475262c11814f6
