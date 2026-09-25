MAKEFLAGS=--no-builtin-rules --no-builtin-variables --always-make

fmt:
	golangci-lint fmt

lint:
	golangci-lint cache clean && golangci-lint run

build:
	go build -v ./...

test:
	go test -race -v ./...

compat:
	go tool gorelease

# endpoint を使わない（ローカルスキーマの）example を再生成する
# gqlgen は Go 名をプロセス全体で保持するため、スキーマの異なる example は 1 config 1 プロセスで実行する
generate-examples:
	for dir in $$(grep -L '^endpoint:' example/*/.gqlgenc.yml | xargs -n1 dirname); do \
		go run . -c $$dir || exit 1; \
	done
