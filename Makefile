.PHONY: build check demo format install test

build:
	go build -o ./ai-meter ./cmd/ai-meter

check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
	go test -race ./...

demo:
	go run ./cmd/ai-meter --demo

format:
	gofmt -w .

install:
	go install ./cmd/ai-meter

test:
	go test ./...
