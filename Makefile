.PHONY: dev build generate test lint clean

dev:
	go run cmd/server/main.go -config configs/config.yaml

build:
	go build -o bin/server cmd/server/main.go

generate:
	go generate ./...

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/
