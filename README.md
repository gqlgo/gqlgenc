# gqlgenc

## What is gqlgenc ?

This is Go library for building GraphQL client with [gqlgen](https://github.com/99designs/gqlgen)

### Query First, not Code First

However, as I work with [gqlgen](https://github.com/99designs/gqlgen) and [graphql-code-generator](https://graphql-code-generator.com/) every day, I find out the beauty of automatic generation.
So I want to automatically generate types.

### based on gqlgen

- [Khan/genqlient](https://github.com/Khan/genqlient) is built from scratch as its own system. However, since gqlgenc is based on gqlgen, knowledge learned from either can be directly applied.

## Usage

```shell script
go get -tool github.com/Yamashou/gqlgenc/v3@latest
or
go install github.com/Yamashou/gqlgenc/v3@latest
```

## How to use

### Client Codes Only

```yaml
# schema for query
schema:
  - ""
endpoint:
  url: https://api.annict.com/graphql # Where do you want to send your request?
  headers: # If you need header for getting introspection query, set it
    Authorization: "Bearer ${ANNICT_KEY}" # support environment variables
# client to generate
client:
  package: generated
  filename: ./client.go # Where should any generated client go?
# query to generate
query:
  source: "./query/*.graphql" # Where are all the query files located?
  package: generated
  filename: ./client.go # Where should any generated client go?
# query to generate
model:
 package: generated
 filename: ./models_gen.go # https://github.com/99designs/gqlgen/tree/master/plugin/modelgen
models:
  Int:
    model: github.com/99designs/gqlgen/graphql.Int64
  Date:
    model: github.com/99designs/gqlgen/graphql.Time
federation: # Add this if your schema includes Apollo Federation related directives
  version: 2
# input model config
nullable_input_omittable: true
enable_model_json_omitzero: true
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

	"github.com/Yamashou/gqlgenc/v3/clientgen"

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

	clientPlugin := clientgen.New(queries, clientPackage, nil)
	err = api.Generate(cfg,
		api.AddPlugin(clientPlugin),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}
}
```

## Code Generation Flow

The following sequence diagram illustrates the code generation flow of gqlgenc:

```mermaid
sequenceDiagram
    actor User
    participant Main as main.go
    participant Run as run.go
    participant Config as config
    participant Client as client
    participant Introspection as introspection
    participant QueryParser as queryparser
    participant CodeGen as codegen
    participant ModelGen as modelgen
    participant QueryGen as querygen
    participant ClientGen as clientgen

    User->>Main: Execute gqlgenc command
    Main->>Run: run()

    Note over Run,Config: Configuration Loading (TestLoadConfig)
    Run->>Config: FindConfigFile(".gqlgenc.yml")
    Config-->>Run: config file path

    Run->>Config: LoadConfig(cfgFile)
    Note right of Config: [TestLoadConfig]<br/>Read config file<br/>Parse YAML with env expansion

    alt Config validation error
        Config->>Config: Check schema/endpoint
        Note right of Config: Error if both specified<br/>Error if neither specified
        Config-->>Run: Error
    else Config validation success
        Config->>Config: Validate model config
        Config->>Config: Set schema filenames (glob)
        Config->>Config: Set default gqlgen fields
        Note right of Config: Directives, Exec, Resolver,<br/>Federation configs
        Config-->>Run: Config
    end

    Note over Run,Introspection: Schema Loading & Initialization (TestLoadSchema)
    Run->>Config: LoadSchema(ctx)

    alt Local Schema
        Config->>Config: GQLGenConfig.LoadSchema()
        Note right of Config: [TestLoadSchema]<br/>Load from local files
        Config-->>Config: GraphQL AST
    else Remote Schema (Endpoint)
        Config->>Client: Use endpoint.Client or http.DefaultClient
        Client-->>Config: HTTP Client
        Config->>Introspection: introspectionSchema(ctx, client, URL, headers)
        Introspection->>Client: Post(introspectionQuery)

        alt HTTP Error
            Client-->>Introspection: Error (500, 404, etc)
            Introspection-->>Config: Error
            Config-->>Run: Error
        else Success
            Client-->>Introspection: Schema Response
            Introspection->>Introspection: Parse introspection response

            alt Invalid Schema
                Introspection->>Introspection: Validate schema
                Introspection-->>Config: Validation Error
                Config-->>Run: Error
            else Valid Schema
                alt Query type is null
                    Introspection->>Introspection: Initialize default Query type
                end
                Introspection-->>Config: GraphQL AST
                Config->>Config: Set GQLGenConfig.Schema
            end
        end
    end

    Config->>Config: Delete existing generated files
    Note right of Config: Unlink model, query,<br/>client gen files
    Config->>Config: Initialize Models & StructTag
    Note right of Config: Set default Models map<br/>Set default StructTag="json"
    Config->>Config: GQLGenConfig.Init()
    Config->>Config: Sort Schema.Implements
    Note right of Config: [TestLoadSchema]<br/>Sort interface implementations<br/>for deterministic output
    Config-->>Run: Initialized Config

    Note over Run,QueryParser: Query File Parsing (TestLoadQuery)
    Run->>Config: GQLGencConfig.LoadQuery(schema)
    Config->>QueryParser: LoadQuerySources(query files)

    alt Query file not found or syntax error
        QueryParser-->>Config: Error
        Config-->>Run: Error
    else Query files loaded
        QueryParser-->>Config: Query Sources
        Config->>QueryParser: QueryDocument(schema, sources)
        Note right of QueryParser: [TestLoadQuery]<br/>Parse & validate<br/>GraphQL queries

        alt Query validation error
            QueryParser-->>Config: Error (unknown field, etc)
            Config-->>Run: Error
        else Valid queries
            QueryParser-->>Config: Query Document
            Config->>QueryParser: OperationQueryDocuments()
            Note right of QueryParser: [TestLoadQuery]<br/>Split by operation<br/>(query/mutation/subscription)
            QueryParser-->>Config: Operation Documents
            Config->>Config: Set QueryDocument & OperationQueryDocuments
            Config-->>Run: Success
        end
    end

    Note over Run,ClientGen: Code Generation
    Run->>Run: plugins.GenerateCode(cfg)

    Note over Run,ModelGen: Model Generation (gqlgen based)
    Run->>ModelGen: New(cfg, operationDocs)
    Run->>ModelGen: MutateConfig()
    ModelGen->>ModelGen: Generate models/enums/scalars
    Note right of ModelGen: Generate Go types<br/>for GraphQL types
    ModelGen-->>Run: models_gen.go

    Note over Run,CodeGen: Go Type Analysis
    Run->>CodeGen: CreateOperations(queryDoc, opDocs)
    Note right of CodeGen: Create operation structs<br/>with args & variables
    CodeGen-->>Run: Operations
    Run->>CodeGen: CreateGoTypes(operations)
    Note right of CodeGen: Convert GraphQL types<br/>to Go types
    CodeGen-->>Run: Go Types

    Note over Run,QueryGen: Query Code Generation
    Run->>QueryGen: New(cfg, operations, goTypes)
    Run->>QueryGen: MutateConfig()
    QueryGen->>QueryGen: RenderTemplate(template.tmpl)
    Note right of QueryGen: Generate response types<br/>& unmarshal code
    QueryGen-->>Run: query code with types

    Note over Run,ClientGen: Client Code Generation
    Run->>ClientGen: New(cfg, operations)
    Run->>ClientGen: MutateConfig()
    ClientGen->>ClientGen: RenderTemplate(template.tmpl)
    Note right of ClientGen: Generate client methods<br/>for each operation
    ClientGen-->>Run: client.go with methods

    Run-->>Main: Success
    Main-->>User: Generated code files
```

### Flow Description

1. **Configuration Loading** (`config.LoadConfig()` - covered by `TestLoadConfig`):
   - Find and load `.gqlgenc.yml` configuration file
   - Read and parse YAML configuration with environment variable expansion (`os.ExpandEnv`)
   - **Validation checks**:
     - Error if both `schema` and `endpoint` are specified
     - Error if neither `schema` nor `endpoint` is specified
     - Error if `clientgen` is set but `querygen` is not set
     - Validate model configuration
   - Set schema filenames using glob patterns (e.g., `schema/**/*.graphql`)
   - Set default gqlgen fields (Directives, Exec, Resolver, Federation)
   - **Platform-specific handling**:
     - Windows: Use backslash path separators
     - Non-Windows: Use forward slash path separators

2. **Schema Loading & Initialization** (`config.LoadSchema()` - covered by `TestLoadSchema`):
   - **Local Schema Path**:
     - Load schema from local GraphQL files using glob patterns
     - Parse GraphQL SDL files into AST
   - **Remote Schema Path** (Endpoint):
     - Use custom HTTP client if provided (`endpoint.Client`), or `http.DefaultClient`
     - Execute GraphQL introspection query to fetch schema
     - **Error handling**:
       - HTTP errors (500, 404, etc.) are propagated with context
       - Invalid schema responses trigger validation errors
     - Parse introspection response and build GraphQL AST
     - Initialize default Query type if `schema.queryType` is null
   - **Post-Schema Processing**:
     - Delete existing generated files to ensure clean generation (model, query, client)
     - Initialize gqlgen config (`Models` map, `StructTag` default to "json")
     - Execute `GQLGenConfig.Init()` to prepare schema
     - Sort interface implementations (`Schema.Implements`) for deterministic output

3. **Query Parsing** (`GQLGencConfig.LoadQuery()` - covered by `TestLoadQuery`):
   - Load GraphQL query files from specified paths
   - **Error handling**:
     - Query file not found errors
     - GraphQL syntax errors (e.g., missing closing braces)
     - Schema validation errors (e.g., querying non-existent fields)
   - Parse and validate GraphQL queries against the loaded schema
   - Split queries into operation documents by type (query/mutation/subscription)
   - Store parsed `QueryDocument` and `OperationQueryDocuments` in config
   - **Empty query list handling**: Returns success with empty operation documents

4. **Code Generation**:
   - **Model Generation**: Generate Go types for GraphQL scalars, enums, and input types
   - **Operation & Type Generation**: Convert GraphQL operations to Go operation structures and response types
   - **Query Generation**: Generate Go code for query/mutation response types
   - **Client Generation**: Generate client methods for executing queries/mutations

### Test Coverage

The following unit tests ensure the reliability of each phase:

- **`TestLoadConfig`**: Tests configuration loading, validation, and platform-specific path handling
  - Config file parsing (YAML with environment variables)
  - Validation errors (both/neither schema and endpoint)
  - Unknown keys detection
  - Platform-specific glob pattern resolution (Windows vs non-Windows)

- **`TestLoadSchema`**: Tests schema loading from both local and remote sources
  - Local schema loading from files
  - Remote schema via introspection query
  - HTTP error handling
  - Invalid schema validation
  - Query type initialization when null
  - Interface implementation sorting

- **`TestLoadQuery`**: Tests query file parsing and validation
  - Single and multiple query file loading
  - Empty query list handling
  - Syntax error detection
  - Schema validation (non-existent field references)

## VS

