.PHONY: fmt build vet test race check

fmt:
	gofmt -w internal

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

check: build vet test race
	test -z "$$(gofmt -l internal)"
	git diff --check
