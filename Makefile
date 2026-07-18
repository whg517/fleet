.PHONY: dev build generate test lint clean ent-gen

ent-gen:
	go run ./cmd/ent-gen

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
