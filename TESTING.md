# Run tests

To run tests simply run

```shell script
go test ./... -coverprofile=coverage.out
```

To deep dive into test coverage, run the following command to see the result in your terminal

```shell script
go tool cover -func=coverage.out
```

or the following to see the result in your browser

```shell script
go tool cover -html=coverage.out
```


# End-to-End Testing

The `generator` package contains golden tests which
run the full generator pipeline
and compare the generated code with checked-in files.

Every directory in `generator/testdata` is a test case,
except `multi_config`, which is used by other tests.
A test case has its own config file (e.g. `gqlgenc.yml`),
the schema and query files the config points to,
and an `expected` directory with the generated files.
The generated code is written to `actual`, which is ignored by git.
The test checks that the code in `actual` compiles
and that `actual` and `expected` hold the same files with the same content.

To run the golden tests:

```shell script
go test ./generator -run TestSuite/TestGenerator_withTestData
```

To add a test case,
copy one of the directories in `generator/testdata`,
modify the config, schema and query files to express your test case,
and create its `expected` files as described below.

To regenerate the `expected` files after changing a test case or the generator, run

```shell script
make golden-update
```

which runs the golden tests with the `-update` flag.
It replaces the `expected` directory of every test case with the generated output,
so files that are no longer generated are removed.
Review the changes with `git diff` before committing them.
