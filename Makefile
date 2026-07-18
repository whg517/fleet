.PHONY: dev build generate test lint clean ent-gen web-build web-lint web-test

ent-gen:
	go run ./cmd/ent-gen

dev:
	go run cmd/server/main.go -config configs/config.yaml

build:
	go build -o bin/server cmd/server/main.go

generate:
	go generate ./...

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

clean:
	rm -rf bin/

web-build:
	cd web && npm run build

web-lint:
	cd web && npm run lint

web-test:
	cd web && npm run test -- --passWithNoTests

install-hooks:
	cp scripts/hooks/pre-commit .git/hooks/pre-commit
	cp scripts/hooks/commit-msg .git/hooks/commit-msg
	chmod +x .git/hooks/pre-commit .git/hooks/commit-msg
	@echo "Git hooks installed"
