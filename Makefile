MAKEFLAGS=--no-builtin-rules --no-builtin-variables --always-make

# e2e で使うダミーの GraphQL サーバー。タグがないのでコミットで固定する
GQLDUMMY_VERSION = 6f79240ac8e2e44e7e80bcc0dcc784cf8349db47
GQLDUMMY_PORT ?= 18080

fmt:
	golangci-lint fmt

lint:
	golangci-lint cache clean && golangci-lint run

build:
	go build -v ./...

test:
	go test -race -v ./...

install-gqldummy:
	go install github.com/Yamashou/gqldummy@$(GQLDUMMY_VERSION)

# e2e_test.go を持つ fixture ごとに、その fixture のスキーマで gqldummy を起動して e2e_test.go を実行する
# GQLDUMMY_URL がないと e2e_test.go は skip するので、make test では実行されない
e2e:
	for dir in $$(ls generator/testdata/*/e2e_test.go | xargs -n1 dirname); do \
		gqldummy serve --port $(GQLDUMMY_PORT) $$dir/schemas/*.graphql & pid=$$!; \
		curl --silent --output /dev/null --retry 10 --retry-connrefused --retry-delay 1 http://localhost:$(GQLDUMMY_PORT)/; ready=$$?; \
		if [ $$ready -eq 0 ]; then \
			GQLDUMMY_URL=http://localhost:$(GQLDUMMY_PORT)/graphql go test -count=1 -v ./$$dir/; status=$$?; \
		else \
			echo "gqldummy did not start for $$dir"; status=1; \
		fi; \
		kill $$pid; wait $$pid 2>/dev/null; \
		[ $$status -eq 0 ] || exit $$status; \
	done

golden-update:
	go test ./generator -run TestSuite/TestGenerator_withTestData -update

compat:
	go tool gorelease

# endpoint を使わない（ローカルスキーマの）example を再生成する
# gqlgen は Go 名をプロセス全体で保持するため、スキーマの異なる example は 1 config 1 プロセスで実行する
generate-examples:
	for dir in $$(grep -L '^endpoint:' example/*/.gqlgenc.yml | xargs -n1 dirname); do \
		go run . -c $$dir || exit 1; \
	done
