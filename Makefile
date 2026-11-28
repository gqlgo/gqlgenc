MAKEFLAGS=--no-builtin-rules --no-builtin-variables --always-make

fmt:
	go tool golangci-lint fmt

lint:
	go tool golangci-lint run

build:
	go build -v ./...

test:
	go test -v ./...
