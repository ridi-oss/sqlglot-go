# sqlglot-go — common developer tasks. Toolchain via mise (see mise.toml).
.PHONY: all check build test test-race vet fmt fmt-check lint tidy reference

all: check

## check: build + vet + gofmt-check + race tests (the pre-push gate)
check: build vet fmt-check test-race

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "these files need gofmt:"; echo "$$out"; exit 1; fi

## lint: golangci-lint (install: https://golangci-lint.run)
lint:
	golangci-lint run

## tidy: sync go.mod/go.sum
tidy:
	go mod tidy

## reference: fetch the pinned upstream sqlglot source into .reference/ (gitignored)
reference:
	./scripts/fetch-reference.sh
