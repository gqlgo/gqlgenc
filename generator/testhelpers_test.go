package generator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test helpers shared by the internal tests of this package and the external
// generator_test package: they are exported so that both can use them, and
// live in a _test.go file so that they are not part of the package.

// HelperConfigDirsEnv makes the test binary generate the listed configs
// (separated by the PATH list separator) in one GenerateAll call and exit, so
// that a test can run configs in a fresh process.
const HelperConfigDirsEnv = "GQLGENC_TEST_GENERATE_DIRS"

// realGo is the go binary that was in PATH when the test binary started,
// resolved before any test puts a wrapper in front of it, so that wrappers do
// not chain.
var realGo, realGoErr = exec.LookPath("go")

// TestGenerateAllHelperProcess is not a test: when HelperConfigDirsEnv is set
// it generates those configs in this process and exits, so that
// GenerateInFreshProcess starts with fresh process-wide state (the gqlgen
// package cache, its table of chosen Go model names, and so on), untouched by
// the other tests of this binary.
func TestGenerateAllHelperProcess(t *testing.T) {
	dirs := os.Getenv(HelperConfigDirsEnv)
	if dirs == "" {
		t.Skip("helper process only")
	}

	err := GenerateAll(context.Background(), strings.Split(dirs, string(os.PathListSeparator)))
	if err != nil {
		t.Fatal(err)
	}
}

// GenerateInFreshProcess runs GenerateAll for dirs in a fresh process started
// in workDir, as a gqlgenc invocation would, and fails the test when that
// process fails. The child inherits the environment, so a go wrapper put in
// PATH by the test counts the child's go invocations too.
//
// Arguments:
//   - workDir: the working directory of the child; "" keeps the current one
//   - dirs: the config directories, absolute or relative to workDir
//
// Returns:
//   - nothing
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the child process passed TestGenerateAllHelperProcess
func GenerateInFreshProcess(t *testing.T, workDir string, dirs ...string) {
	t.Helper()

	// The test binary is addressed by its absolute path, because a relative
	// os.Args[0] would be resolved against workDir.
	executable, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(executable, "-test.run=^TestGenerateAllHelperProcess$", "-test.v")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), HelperConfigDirsEnv+"="+strings.Join(dirs, string(os.PathListSeparator)))

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%v: %s", dirs, out)
	require.Contains(t, string(out), "--- PASS: TestGenerateAllHelperProcess", "%v: %s", dirs, out)
}

// CountGoListCalls runs fn with a go wrapper first in PATH that records every
// invocation, and returns how many go list invocations named each of
// importPaths on their command line. The wrapper delegates to the go binary
// found when the test binary started, so calling this twice in one test does
// not chain wrappers.
//
// Arguments:
//   - importPaths: the packages to count
//   - fn: the code to run under the wrapper; child processes it starts
//     inherit the wrapper
//
// Returns:
//   - map[string]int: per import path, the number of go list invocations
//     whose arguments contain it
//
// Preconditions:
//   - /bin/sh is available
//
// Postconditions:
//   - PATH is restored when the test ends
func CountGoListCalls(t *testing.T, importPaths []string, fn func()) map[string]int {
	t.Helper()
	require.NoError(t, realGoErr, "go binary not found in PATH")

	binDir := t.TempDir()
	logFile := filepath.Join(binDir, "go.log")

	wrapper := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logFile + "\"\nexec \"" + realGo + "\" \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "go"), []byte(wrapper), 0o755))

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	fn()

	counts := map[string]int{}

	logged, err := os.ReadFile(logFile)
	if os.IsNotExist(err) {
		return counts
	}

	require.NoError(t, err)

	for line := range strings.SplitSeq(string(logged), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "list" {
			continue
		}

		for _, importPath := range importPaths {
			if slices.Contains(fields[1:], importPath) {
				counts[importPath]++
			}
		}
	}

	return counts
}
