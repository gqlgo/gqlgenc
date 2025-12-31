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
gqlgenc generate --configdir schemas
```

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

## Documents

- [How to configure gqlgen using gqlgen.yml](https://gqlgen.com/config/)
- [How to write plugins for gqlgen](https://gqlgen.com/reference/plugins/)


## Comments

### Japanese Comments
These codes have Japanese comments. Replace with English.

### Subscription

This client does not support subscription. If you need a subscription, please create an issue or pull request.

### Pre-conditions

[clientgenv2](https://github.com/gqlgo/gqlgenc/tree/master/clientgenv2) is created based on [modelgen](https://github.com/99designs/gqlgen/tree/master/plugin/modelgen). So if you don't have a modelgen, it may be a mysterious move.
