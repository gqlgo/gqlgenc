MAKEFLAGS=--no-builtin-rules --no-builtin-variables --always-make

fmt:
	goimports -local github.com/gqlgo/gqlgenc -w . && gofumpt -extra -w . && gci write -s Standard -s Default .

lint:
	golangci-lint cache clean && golangci-lint run

test:
	go test -v ./...

compat:
	go tool gorelease
