.PHONY: build test vet

build:
	go build -o dotfiles-mcp ./...

test:
	go test ./... -count=1

vet:
	go vet ./...

-include $(HOME)/hairglasses-studio/dotfiles/make/pipeline.mk
