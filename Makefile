.PHONY: run build test lint fmt

run:
	go run .

build:
	go build -o goxcms .

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

fmt:
	gofmt -w .
