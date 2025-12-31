package main

import (
	"fmt"
	"os"

	"github.com/gqlgo/gqlgenc/config"
	"github.com/gqlgo/gqlgenc/generator"
	"github.com/urfave/cli/v2"
)

var version = "0.33.0"

var versionCmd = &cli.Command{
	Name:  "version",
	Usage: "print the version",
	Action: func(ctx *cli.Context) error {
		fmt.Println(version)
		return nil
	},
}

var generateCmd = &cli.Command{
	Name:  "generate",
	Usage: "generate a graphql client based on schema",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "configdir, c", Usage: "the directory with configuration file", Value: "."},
	},
	Action: func(ctx *cli.Context) error {
		configDir := ctx.String("configdir")
		cfg, err := config.LoadConfigFromDefaultLocations(configDir)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%+v\n", err.Error())
			os.Exit(2)
		}

		if err := generator.Generate(ctx.Context, cfg); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%+v\n", err.Error())
			os.Exit(4)
		}

		return nil
	},
}

func main() {
	app := cli.NewApp()
	app.Name = "gqlgenc"
	app.Description = "This is a library for quickly creating strictly typed graphql client in golang"
	app.Usage = generateCmd.Usage
	app.DefaultCommand = "generate"
	app.Commands = []*cli.Command{
		versionCmd,
		generateCmd,
	}

	if err := app.Run(os.Args); err != nil {
		_, _ = fmt.Fprint(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}
