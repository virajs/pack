package main

import (
	"os"

	"github.com/buildpack/pack"
	"github.com/spf13/cobra"
)

func main() {
	wd, _ := os.Getwd()

	var buildFlags pack.BuildFlags
	buildCommand := &cobra.Command{
		Use:  "build <image-name>",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			buildFlags.RepoName = args[0]
			return buildFlags.Run()
		},
	}
	buildCommand.Flags().StringVarP(&buildFlags.AppDir, "path", "p", wd, "path to app dir")
	buildCommand.Flags().StringVar(&buildFlags.BuildImage, "build-image", "packs/build", "build image")
	buildCommand.Flags().StringVar(&buildFlags.RunImage, "run-image", "packs/run", "run image")
	buildCommand.Flags().BoolVar(&buildFlags.Publish, "publish", false, "publish to registry")

	var createBuilderFlags pack.CreateBuilderFlags
	createBuilderCommand := &cobra.Command{
		Use:  "create-builder <image-name> -b <path-to-builder-toml>",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			createBuilderFlags.RepoName = args[0]
			return createBuilderFlags.Run()
		},
	}
	createBuilderCommand.Flags().StringVarP(&createBuilderFlags.RepoName, "path", "p", wd, "path to app dir")
	createBuilderCommand.Flags().StringVarP(&createBuilderFlags.BuilderTomlPath, "builder-config", "b", wd, "path to builder.toml file")

	rootCmd := &cobra.Command{Use: "pack"}
	rootCmd.AddCommand(buildCommand)
	rootCmd.AddCommand(createBuilderCommand)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
