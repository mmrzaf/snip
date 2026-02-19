
set shell := ["bash", "-cu"]

build:
    mkdir -p bin
    go build -o bin/snip ./cmd/snip

test:
    go test ./...

fmt:
    gofmt -w .

lint:
    golangci-lint run

run *args:
    go run ./cmd/snip {{args}}

