# gqlgenc

## What is gqlgenc ?

This is Go library for building GraphQL client with [gqlgen](https://github.com/99designs/gqlgen)

## Motivation

Now, if you build GraphQL api client for Go, have choice:

 - [github.com/shurcooL/graphql](https://github.com/shurcooL/graphql)
 - [github.com/machinebox/graphql](https://github.com/machinebox/graphql)

These libraries are very simple and easy to handle.
However, as I work with [gqlgen](https://github.com/99designs/gqlgen) and [graphql-code-generator](https://graphql-code-generator.com/) every day, I find out the beauty of automatic generation.
So I want to automatically generate types.

## Installation

```shell script
go get -u github.com/gqlgo/gqlgenc
```

## How to use

### Client Codes Only

gqlgenc base is gqlgen with [plugins](https://gqlgen.com/reference/plugins/). So the setting is yaml in each format.
gqlgenc can be configured using a `.gqlgenc.yml` file

Load a schema from a remote server:

```yaml
model:
  package: generated
  filename: ./models_gen.go # https://github.com/99designs/gqlgen/tree/master/plugin/modelgen
client:
  package: generated
  filename: ./client.go # Where should any generated client go?
models:
  Int:
    model: github.com/99designs/gqlgen/graphql.Int64
  Date:
    model: github.com/99designs/gqlgen/graphql.Time
federation: # Add this if your schema includes Apollo Federation related directives
  version: 2
endpoint:
  url: https://api.annict.com/graphql # Where do you want to send your request?
  headers: # If you need header for getting introspection query, set it
    Authorization: "Bearer ${ANNICT_KEY}" # support environment variables
query:
  - "./query/*.graphql" # Where are all the query files located?
generate:
  clientInterfaceName: "GithubGraphQLClient" # Determine the name of the generated client interface
  structFieldsAlwaysPointers: true # Always use pointers for struct fields (default: true)  [same as gqlgen](https://github.com/99designs/gqlgen/blob/e1ef86e795e738654c98553b325a248c02c8c2f8/docs/content/config.md?plain=1#L73)
```

Load a schema from a local file:

```yaml
model:
  package: generated
  filename: ./models_gen.go # https://github.com/99designs/gqlgen/tree/master/plugin/modelgen
client:
  package: generated
  filename: ./client.go # Where should any generated client go?
models:
  Int:
    model: github.com/99designs/gqlgen/graphql.Int64
  Date:
    model: github.com/99designs/gqlgen/graphql.Time
federation: # Add this if your schema includes Apollo Federation related directives
  version: 2
schema:
  - "schema/**/*.graphql" # Where are all the schema files located?
query:
  - "./query/*.graphql" # Where are all the query files located?
generate:
  clientInterfaceName: "GithubGraphQLClient" # Determine the name of the generated client interface
  structFieldsAlwaysPointers: true # Optional: Always use pointers for struct fields (default: true). [same as gqlgen](https://github.com/99designs/gqlgen/blob/e1ef86e795e738654c98553b325a248c02c8c2f8/docs/content/config.md?plain=1#L73)
  onlyUsedModels: true # Optional: Only generate used models
  enableClientJsonOmitemptyTag: true # Optional: Controls whether the "omitempty" option is added to JSON tags (default: true)
```

Execute the following command on same directory for .gqlgenc.yml

```shell script
gqlgenc
```

or if you want to specify a different directory where .gqlgenc.yml file resides
(e.g. in this example the directory is *schemas*):

```shell script
gqlgenc -c schemas
```

`-c` is the shorthand of `--configdir`. The config file is searched in the
directory and then in its parents, and gqlgenc changes into its directory
before reading it, so every relative path in a config (`schema`, `query`,
`model.filename`, `client.filename`, relative import paths in `autobind` and
`models`) is relative to the config file. Up to v0.40.x they were relative to
the directory gqlgenc was started in.

#### Several configs in one process

`-c` can be repeated. The configs are generated in the given order in one
process, and the packages named in `autobind` and `models` are loaded and
type checked once for the whole run instead of once per config:

```shell script
gqlgenc -c clients/orders -c clients/products -c clients/customers
```

- The configs are processed one after another. To use more cores, run one
  process per group of configs, for example with `xargs -P`.
- The configs must belong to the same Go module, because the cache is keyed
  by import path.
- A package named in `autobind` or `models` must not import the output of
  another config of the same run; only the output packages themselves are
  dropped from the cache when they are rewritten.
- Errors are reported as `<configdir>: <error>`.
- Put configs of one schema in one process. gqlgen keeps the Go names it has
  chosen for GraphQL names for the whole process, so with different schemas a
  GraphQL name whose Go name is already taken (`Foo_Bar` after `FooBar`) gets
  a suffix (`FooBar0`) that a separate run would not add.
- A config may bind the output of an earlier config in the same run.

### With gqlgen

Do this when creating a server and client for Go.
You create your own entrypoint for gqlgen.
This use case is very useful for testing your server.


```go
package main

import (
	"fmt"
	"os"

	"github.com/gqlgo/gqlgenc/clientgenv2"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
)

func main() {
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config", err.Error())
		os.Exit(2)
	}
	queries := []string{"client.query", "fragemt.query"}
	clientPackage := config.PackageConfig{
		Filename: "./client.go",
		Package:  "gen",
	}

	clientPlugin := clientgenv2.New(queries, clientPackage, nil)
	err = api.Generate(cfg,
		api.AddPlugin(clientPlugin),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}
}
```

## Development

Tool versions are managed with [mise](https://mise.jdx.dev/) and pinned in `.mise.toml` / `mise.lock`.
Activate mise in your shell (`mise activate`, see the [getting started guide](https://mise.jdx.dev/getting-started.html)) or prefix commands with `mise exec --` so the pinned binaries are on `PATH`.

```shell script
mise install
make fmt
make lint
make test
```

## Documents

- [How to configure gqlgen using gqlgen.yml](https://gqlgen.com/config/)
- [How to write plugins for gqlgen](https://gqlgen.com/reference/plugins/)


## Comments

### Japanese Comments
These codes have Japanese comments. Replace with English.

### Encoding of request variables

`clientv2` encodes request bodies with `encoding/json/v2` configured with `encoding/json` v1 semantics, plus the gqlgen conventions: values implementing `graphql.ContextMarshaler` or `graphql.Marshaler` are encoded with `MarshalGQLContext` / `MarshalGQL`, which take precedence over `json.Marshaler` and `encoding.TextMarshaler`. A nil value of such a type (a nil pointer, or a nil named slice, map or interface such as `type IDs []string`) is encoded as `null` without calling the method, so an optional list variable left nil reaches the server as `null` rather than `[]`; `Options.EncodeNilSliceAsEmptyArray` calls the method on nil slices instead. This also applies to a non-nil pointer to a nil slice or map (`&IDs(nil)`), which before v0.41.0 called the method and produced `[]`. Everything else is encoded exactly as `encoding/json` would encode it.

Before v0.40.0, `clientv2` used its own reflect-based encoder. Its output differs from the current one in the following ways, so upgrading is a breaking change if you relied on them:

- `[]byte` and other byte slices are encoded as base64 strings, not as arrays of numbers. Use `[]int` or a type implementing `MarshalGQL` if the server expects an array.
- Fields of embedded structs are flattened into the parent object instead of being nested under the type name.
- Struct fields are emitted in declaration order instead of alphabetical order. The JSON is equivalent, but byte-for-byte comparisons of the encoded output need updating.

### Subscription

This client does not support subscription. If you need a subscription, please create an issue or pull request.

### Pre-conditions

[clientgenv2](https://github.com/gqlgo/gqlgenc/tree/master/clientgenv2) is created based on [modelgen](https://github.com/99designs/gqlgen/tree/master/plugin/modelgen). So if you don't have a modelgen, it may be a mysterious move.
