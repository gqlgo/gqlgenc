package clientgenv2

import (
	"testing"

	"github.com/99designs/gqlgen/codegen/config"

	gqlgencConfig "github.com/gqlgo/gqlgenc/config"
)

func TestNew_NilGenerateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		generateConfig *gqlgencConfig.GenerateConfig
	}{
		{
			name:           "success: nil generate config is normalized to empty struct",
			generateConfig: nil,
		},
		{
			name:           "success: non-nil generate config is preserved",
			generateConfig: &gqlgencConfig.GenerateConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := New(nil, nil, config.PackageConfig{}, tt.generateConfig)
			if p.GenerateConfig == nil {
				t.Fatal("Plugin.GenerateConfig must not be nil")
			}
		})
	}
}

func TestNewWithQueryDocument_NilGenerateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		generateConfig *gqlgencConfig.GenerateConfig
	}{
		{
			name:           "success: nil generate config is normalized to empty struct",
			generateConfig: nil,
		},
		{
			name:           "success: non-nil generate config is preserved",
			generateConfig: &gqlgencConfig.GenerateConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := NewWithQueryDocument(nil, config.PackageConfig{}, tt.generateConfig)
			if p.GenerateConfig == nil {
				t.Fatal("Plugin.GenerateConfig must not be nil")
			}
		})
	}
}

func TestNewSourceGenerator_NilGenerateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		generateConfig *gqlgencConfig.GenerateConfig
	}{
		{
			name:           "success: nil generate config is normalized to empty struct",
			generateConfig: nil,
		},
		{
			name:           "success: non-nil generate config is preserved",
			generateConfig: &gqlgencConfig.GenerateConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sg := NewSourceGenerator(&config.Config{}, config.PackageConfig{}, tt.generateConfig)
			if sg.generateConfig == nil {
				t.Fatal("SourceGenerator.generateConfig must not be nil")
			}
		})
	}
}

func TestNewSource_NilGenerateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		generateConfig *gqlgencConfig.GenerateConfig
	}{
		{
			name:           "success: nil generate config is normalized to empty struct",
			generateConfig: nil,
		},
		{
			name:           "success: non-nil generate config is preserved",
			generateConfig: &gqlgencConfig.GenerateConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := NewSource(nil, nil, nil, tt.generateConfig)
			if s.generateConfig == nil {
				t.Fatal("Source.generateConfig must not be nil")
			}
		})
	}
}
