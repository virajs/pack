package main

import (
	"os"

	"github.com/buildpack/pack"
	"github.com/spf13/cobra"
)

func main() {
	wd, _ := os.Getwd()

	var appDir, detectImage, stack string
	var publish bool
	v3Build := &cobra.Command{
		Use:  "build [IMAGE NAME]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoName := args[0]
			return pack.Build(appDir, detectImage, repoName, publish)
		},
	}
	v3Build.Flags().StringVarP(&appDir, "path", "p", wd, "path to app dir")
	v3Build.Flags().StringVar(&detectImage, "detect-image", "packs/v3:detect", "detect image")
	v3Build.Flags().BoolVarP(&publish, "publish", "r", false, "publish to registry")

	cfBuild := &cobra.Command{
		Use:  "cf-build [IMAGE NAME]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoName := args[0]
			return pack.CFBuild(appDir, stack, repoName, publish)
		},
	}
	cfBuild.Flags().StringVarP(&appDir, "path", "p", wd, "path to app dir")
	cfBuild.Flags().StringVar(&stack, "stack", "cflinuxfs3", "stack image")
	cfBuild.Flags().BoolVarP(&publish, "publish", "r", false, "publish to registry")

	rootCmd := &cobra.Command{Use: "pack"}
	rootCmd.AddCommand(v3Build)
	rootCmd.AddCommand(cfBuild)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
