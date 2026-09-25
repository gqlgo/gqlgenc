MAKEFLAGS=--no-builtin-rules --no-builtin-variables --always-make

fmt:
	golangci-lint fmt

lint:
	golangci-lint cache clean && golangci-lint run

build:
	go build -v ./...

test:
	go test -race -v ./...

golden-update:
	go test ./generator -run TestSuite/TestGenerator_withTestData -update

compat:
	go tool gorelease
