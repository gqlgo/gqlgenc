package main

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin/federation"

	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/internal/typebind"
	"github.com/Yamashou/gqlgenc/v3/introspection"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// packageCache は、Go のパッケージを読み込んだ結果を持つ。複数の設定を 1 プロセスで処理するときに
// 使い回し、go list と型検査のやり直しを避ける。スレッドセーフではないため、設定は直列に処理する。
type packageCache struct {
	// gqlgen は、直前に bindTypes が作った gqlgen の設定。その Packages (ソースの型検査を伴う
	// パッケージキャッシュ) を次の設定へ引き継ぐために持つ。Packages の型は gqlgen の internal
	// パッケージにあって名前を書けないため、設定ごと持つ。最初の bindTypes までは nil。
	gqlgen *gqlgenconfig.Config
	// binder は export data だけを読む軽い読み込み。autobind と codegen の型の解決に使う。
	binder *typebind.Binder
}

// newPackageCache は、何も読み込んでいない packageCache を返す。
func newPackageCache() *packageCache {
	return &packageCache{gqlgen: nil, binder: typebind.New()}
}

// evict は、出力先 output のパッケージを両方のキャッシュから外し、次に読み込むときに今のファイルから
// 読み直させる。
func (c *packageCache) evict(output *gqlgenconfig.PackageConfig) {
	if c.gqlgen != nil {
		c.gqlgen.Packages.Evict(output.ImportPath())
	}

	if dir, err := filepath.Abs(output.Dir()); err == nil {
		c.binder.EvictDir(dir)
	}
}

// loadSchema は、設定のスキーマを読み込む。schema.endpoint があればサーバーから introspection で
// 取得し、無ければ schema.files のファイルを読む。
//
// 事前条件: cfg は config.LoadConfig が返したもので、Endpoint か SchemaFiles のどちらかを持つ。
func loadSchema(ctx context.Context, cfg *config.Config) (*ast.Schema, error) {
	if cfg.Endpoint != nil {
		schema, err := introspection.LoadRemoteSchema(ctx, cfg.Endpoint.URL, http.Header(cfg.Endpoint.Headers))
		if err != nil {
			return nil, fmt.Errorf("introspect schema failed: %w", err)
		}

		return schema, nil
	}

	sources, err := schemaFileSources(cfg.SchemaFiles)
	if err != nil {
		return nil, err
	}

	// schema.federation.version を指定すると、@key などの federation のディレクティブの定義を
	// スキーマのソースへ足す
	if cfg.FederationVersion != 0 {
		fedPlugin, err := federation.New(cfg.FederationVersion, &gqlgenconfig.Config{})
		if err != nil {
			return nil, fmt.Errorf("failed to create federation plugin: %w", err)
		}

		federationSources, err := fedPlugin.InjectSourcesEarly()
		if err != nil {
			return nil, fmt.Errorf("failed to inject federation directives: %w", err)
		}

		sources = append(sources, federationSources...)
	}

	// gqlgen の LoadSchema は使わない。Packages キャッシュを無条件に作り直し、その場でプリロード用の
	// go list が走るため、複数の設定で使い回しているキャッシュが設定ごとに使い捨てになる。
	// 本質的な処理は gqlparser.LoadSchema と Query 型の補完だけなので、ここで直接行う。
	// 検証は bindTypes の Init() が行う。
	schema, err := gqlparser.LoadSchema(sources...)
	if err != nil {
		return nil, fmt.Errorf("load local schema failed: %w", err)
	}
	if schema.Query == nil {
		schema.Query = &ast.Definition{Kind: ast.Object, Name: "Query"}
		schema.Types["Query"] = schema.Query
	}

	return schema, nil
}

// schemaFileSources は、スキーマのファイルを読み、gqlparser のソースにする。
func schemaFileSources(schemaFilenames []string) ([]*ast.Source, error) {
	sources := make([]*ast.Source, 0, len(schemaFilenames))

	for _, schemaFilename := range schemaFilenames {
		schemaFilename = filepath.ToSlash(schemaFilename)

		schemaRaw, err := os.ReadFile(schemaFilename)
		if err != nil {
			return nil, fmt.Errorf("unable to open schema: %w", err)
		}

		sources = append(sources, &ast.Source{Name: schemaFilename, Input: string(schemaRaw)})
	}

	return sources, nil
}

// bindTypes は、スキーマの型を Go の型へ束縛する。gqlgen の設定を組み立てて Init() を実行し、
// bind.type.packages のパッケージから同じ名前の型を束縛する。戻り値の gqlgen の設定は、束縛の結果
// (Models)・スキーマ (Schema)・パッケージキャッシュ (Packages) を持ち、コード生成が使う。
//
// gqlgen の設定は利用者に見せず、v3 が常に使う値をここで固定する。
//
// 事前条件: 前回の生成物 (model / query / client の出力ファイル) は、呼び出し側が出力先から
// 外してある。残っていると、Init() と autobind が前回の生成物の型を束縛先の型として拾う。
// 事後条件: エラーでも、cache.gqlgen は今回の gqlgen の設定になる。
func bindTypes(cfg *config.Config, schema *ast.Schema, cache *packageCache) (*gqlgenconfig.Config, error) {
	// Init() と autobind が束縛を書き足すため、設定の値は写して渡す
	models := maps.Clone(cfg.Models)
	if models == nil {
		models = make(gqlgenconfig.TypeMap)
	}

	// gqlgen が必須にしているが、クライアントの生成では使わない出力先に入れるファイル名
	const unusedGenFilename = "generated.go"

	gqlgenCfg := &gqlgenconfig.Config{
		Schema: schema,
		Model:  cfg.ModelGen,
		Models: models,
		// json/v2 では omitzero が undefined の省略を担うため omitempty は不要。明示的に無効化して
		// model の nullable フィールドのタグを OperationVars と同じ omitzero のみに揃える。
		StructTag:                   "json",
		StructFieldsAlwaysPointers:  false,
		NullableInputOmittable:      true,
		EnableModelJsonOmitzeroTag:  new(true),
		EnableModelJsonOmitemptyTag: new(false),
		Directives:                  make(map[string]gqlgenconfig.DirectiveConfig),
		Exec:                        gqlgenconfig.ExecConfig{Filename: unusedGenFilename},
		Resolver:                    gqlgenconfig.ResolverConfig{Filename: unusedGenFilename},
		Federation:                  gqlgenconfig.PackageConfig{Filename: unusedGenFilename},
	}
	// Init() は Packages が nil のときだけ新しく作るため、前の設定のキャッシュを渡すと
	// go list と型検査が 1 回で済む。生成で書き換わったパッケージは templates.Render が
	// Evict するため、使い回しても安全である。
	if cache.gqlgen != nil {
		gqlgenCfg.Packages = cache.gqlgen.Packages
	}

	// autobind は gqlgen (ソースの型検査を伴う重い読み込み) ではなく、export data だけを読む binder で
	// 行う。codegen の型の解決もこの binder を使う。
	//
	// 束縛先の読み込みと gqlgen の Init() はどちらも go list サブプロセスの待ちが支配的で、
	// 互いに独立している (binder は自前の map だけ、Init は gqlgen の Packages だけに触れる) ため、
	// 並行に実行する。fallback の設定と Autobind は両方が終わってから行う。
	loadErrCh := make(chan error, 1)
	go func() {
		loadErrCh <- cache.binder.Load(slices.Concat(cfg.TypeAutobind, cfg.FragmentAutobind)...)
	}()

	initErr := gqlgenCfg.Init()
	cache.gqlgen = gqlgenCfg

	if err := <-loadErrCh; err != nil {
		return nil, fmt.Errorf("load bound packages failed: %w", err)
	}
	if initErr != nil {
		return nil, fmt.Errorf("generating core failed: %w", initErr)
	}

	// gqlgen が Init で読み込んだパッケージ (graphql / introspection / bind.type.named の束縛先) は
	// fallback で使い回し、二重に読み込まない
	cache.binder.SetFallback(gqlgenCfg.Packages.LoadWithTypes)

	if err := cache.binder.Autobind(schema, gqlgenCfg.Models, cfg.TypeAutobind); err != nil {
		return nil, fmt.Errorf("autobind failed: %w", err)
	}

	// model を生成しない場合は modelgen が動かないため、custom scalar の既定を補う。
	if !cfg.ModelGen.IsDefined() {
		bindDefaultScalars(schema, gqlgenCfg.Models)
	}

	// sort Implements to ensure a deterministic output
	for _, implements := range schema.Implements {
		slices.SortFunc(implements, func(a, b *ast.Definition) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	return gqlgenCfg, nil
}

// bindDefaultScalars は、まだモデルが束縛されていない schema の custom scalar を
// graphql.String に束縛する。これは gqlgen modelgen が非 user-defined scalar に与える既定
// (plugin/modelgen の b.Scalars 束縛) と揃えたもので、modelgen が動かない (model を生成しない)
// 構成でも同じ既定を適用するために使う。
//
// autobind / models: / built-in で既に束縛済みの scalar は UserDefined のため対象外で、
// 対応する型がある場合はその型へのバインドが優先される。
func bindDefaultScalars(schema *ast.Schema, models gqlgenconfig.TypeMap) {
	for _, t := range schema.Types {
		if t.Kind == ast.Scalar && !models.UserDefined(t.Name) {
			models.Add(t.Name, "github.com/99designs/gqlgen/graphql.String")
		}
	}
}
