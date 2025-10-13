MAKEFLAGS=--no-builtin-rules --no-builtin-variables --always-make

# Enable jsonv2 experiment
export GOEXPERIMENT=jsonv2

fmt:
	go tool golangci-lint fmt

lint:
	go tool golangci-lint run

build:
	go build -v ./...

test:
	go test -v ./...
