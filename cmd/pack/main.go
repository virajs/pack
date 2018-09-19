package main

import (
	"github.com/BurntSushi/toml"
	"github.com/buildpack/pack/fs"
	"os"

	"github.com/buildpack/pack"
	"github.com/spf13/cobra"
)

func main() {
	buildCmd := buildCommand()
	createBuilderCmd := createBuilderCommand()

	rootCmd := &cobra.Command{Use: "pack"}
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(createBuilderCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildCommand() *cobra.Command {
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
	buildCommand.Flags().StringVar(&buildFlags.Builder, "builder", "packs/samples", "builder image")
	buildCommand.Flags().StringVar(&buildFlags.RunImage, "run-image", "packs/run", "run image")
	buildCommand.Flags().BoolVar(&buildFlags.Publish, "publish", false, "publish to registry")
	return buildCommand
}

func createBuilderCommand() *cobra.Command {
	var builderTomlPath string
	builderFactory := pack.BuilderFactory{
		FS: &fs.FS{},
	}

	var createBuilderArgs pack.CreateBuilderArgs
	createBuilderCommand := &cobra.Command{
		Use:  "create-builder <image-name> -b <path-to-builder-toml>",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			createBuilderArgs.RepoName = args[0]
			_, err := toml.DecodeFile(builderTomlPath, &createBuilderArgs.Builder)
			if err != nil {
				return err
			}
			stack, err := pack.DefaultStack()
			if err != nil {
				return err
			}
			createBuilderArgs.Stack = stack
			return builderFactory.Create(createBuilderArgs)
		},
	}
	createBuilderCommand.Flags().StringVarP(&builderTomlPath, "builder-config", "b", "", "path to builder.toml file")
	return createBuilderCommand
}
