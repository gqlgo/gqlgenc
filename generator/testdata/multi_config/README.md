# multi_config

Fixture for `GenerateAll` (the repeated `-c` flag). It is not picked up by
`TestGenerator_withTestData` because it has no `gqlgenc.yml` at its root.

The four configs share one base schema and one hand-written `models`
package; `c` and `d` add a schema file of their own. Different schemas are
fine in one run as long as no GraphQL name of a later config maps to a Go
name another GraphQL name of an earlier config already took, which is the
case here.

- `a`: models + client into `a/actual`, autobind `models`
- `b`: models + client into `b/actual`, autobind `models` given as the
  relative import path `../models`, which gqlgenc replaces with the full one
  so that the package is shared
- `c`: models into `b/actual` (the package `b` generated, as
  `models_c_gen.go`) and the client into `c/actual`, autobinding `b/actual`.
  Covers a config that both binds and writes into one package, like clients
  that share one models package
- `d`: models + client into `d/actual`, autobinding `b/actual` (the generated
  output of another config). Its extra
  schema file adds `TagFilter` so that it generates a model of its own

The order is a, b, c, d: `b` must run before `c` and `d`.
