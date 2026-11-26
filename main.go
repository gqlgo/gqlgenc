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
		os.Exit(exitCodeOf(err))
	}
}

// The exit codes of the command. A flag parsing error and a panic also exit
// with 2, which is the default of the flag package and of the Go runtime.
const (
	// exitCodeSuccess means every config was generated.
	exitCodeSuccess = 0
	// exitCodeLoadConfigFailed means a config could not be found or loaded.
	exitCodeLoadConfigFailed = 2
	// exitCodeGenerateFailed means a config was loaded but generating from it failed.
	exitCodeGenerateFailed = 4
)

// exitCodeOf returns the exit code for the result of generator.GenerateAll.
//
// Arguments:
//   - err: the error returned by generator.GenerateAll, or nil
//
// Returns:
//   - int: exitCodeSuccess for nil, exitCodeLoadConfigFailed when err is or wraps a
//     *generator.LoadConfigError, otherwise exitCodeGenerateFailed
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the result is exitCodeSuccess only for a nil err
func exitCodeOf(err error) int {
	if err == nil {
		return exitCodeSuccess
	}

	var loadConfigError *generator.LoadConfigError
	if errors.As(err, &loadConfigError) {
		return exitCodeLoadConfigFailed
	}

	return exitCodeGenerateFailed
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
