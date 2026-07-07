package clientgen

import (
	"fmt"

	"golang.org/x/tools/imports"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin"

	"github.com/Yamashou/gqlgenc/v3/codegen"
	"github.com/Yamashou/gqlgenc/v3/config"
)

var _ plugin.ConfigMutator = &Plugin{}

type Plugin struct {
	cfg        *config.Config
	operations []*codegen.Operation
}

func New(cfg *config.Config, operations []*codegen.Operation) *Plugin {
	return &Plugin{
		cfg:        cfg,
		operations: operations,
	}
}

func (p *Plugin) Name() string {
	return "clientgen"
}

func (p *Plugin) MutateConfig(_ *gqlgenconfig.Config) error {
	if err := RenderTemplate(p.cfg, p.operations); err != nil {
		return fmt.Errorf("template failed: %w", err)
	}

	if _, err := imports.Process(p.cfg.GQLGencConfig.ClientGen.Filename, nil, nil); err != nil {
		return fmt.Errorf("go imports: %w", err)
	}

	return nil
}
