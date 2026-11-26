package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gqlgo/gqlgenc/generator"
)

func TestExitCodeOf(t *testing.T) {
	t.Parallel()

	loadConfigError := &generator.LoadConfigError{ConfigDir: "a", Err: errors.New("not found")}

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "success", err: nil, want: 0},
		{name: "config could not be loaded", err: loadConfigError, want: 2},
		{name: "config could not be loaded, prefixed with its directory", err: fmt.Errorf("a: %w", loadConfigError), want: 2},
		{name: "generation failed", err: errors.New("generating core failed"), want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, exitCodeOf(tt.err))
		})
	}
}

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	t.Run("build override wins", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "v1.2.3", resolveVersion("v1.2.3"))
	})

	t.Run("falls back to the module build info", func(t *testing.T) {
		t.Parallel()

		got := resolveVersion("")
		require.NotEmpty(t, got)
		require.NotEqual(t, "0.33.0", got, "must not report a hardcoded version")
	})
}
