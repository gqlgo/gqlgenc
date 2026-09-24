package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/gqlgo/gqlgenc/generator"
)

// version can be set at build time with -ldflags "-X main.version=v1.2.3".
// When it is empty, the version recorded in the module build info is used,
// which is the requested version for binaries installed with go install.
var version = ""

func main() {
	var (
		showVersion = flag.Bool("version", false, "print the version")
		dirs        []string
	)

	const configDirUsage = "the directory with configuration file; repeat to generate several configs in one process (default \".\")"

	addDir := func(dir string) error {
		dirs = append(dirs, dir)

		return nil
	}

	flag.Func("configdir", configDirUsage, addDir)
	flag.Func("c", configDirUsage+" (shorthand)", addDir)
	flag.Parse()

	if *showVersion {
		fmt.Println(resolveVersion(version))

		return
	}

	err := generator.GenerateAll(context.Background(), dirs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		var loadErr *generator.LoadConfigError
		if errors.As(err, &loadErr) {
			os.Exit(2)
		}

		os.Exit(4)
	}
}

// resolveVersion returns the version to print for -version.
//
// Arguments:
//   - override: the value set at build time with -ldflags, or empty
//
// Returns:
//   - string: override when it is set, otherwise the main module version from
//     the build info, or "(devel)" when no build info is available
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the result is never empty
func resolveVersion(override string) string {
	if override != "" {
		return override
	}

	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "" {
		return info.Main.Version
	}

	return "(devel)"
}
