package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/gqlgo/gqlgenc/config"
	"github.com/gqlgo/gqlgenc/generator"
)

var version = "0.33.0"

func main() {
	var (
		showVersion = flag.Bool("version", false, "print the version")
		configDir   = flag.String("configdir", ".", "the directory with configuration file")
	)

	flag.StringVar(configDir, "c", ".", "the directory with configuration file (shorthand)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)

		return
	}

	cfg, err := config.LoadConfigFromDefaultLocations(*configDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		os.Exit(2)
	}

	ctx := context.Background()

	err = generator.Generate(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		os.Exit(4)
	}
}
