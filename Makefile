MAKEFLAGS=--no-builtin-rules --no-builtin-variables --always-make

fmt:
	goimports -local github.com/gqlgo/gqlgenc -w . && gofumpt -extra -w . && go tool gci write -s Standard -s Default -s "Prefix(github.com/vektah/gqlparser)" -s "Prefix(github.com/99designs/gqlgen)" -s "Prefix(github.com/gqlgo/gqlgenc)" .

lint:
	golangci-lint cache clean && golangci-lint run

test:
	go test -v ./...

compat:
	go tool gorelease
