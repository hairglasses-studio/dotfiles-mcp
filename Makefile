.PHONY: build test vet lint check coverage contract-snapshot contract-check contract-diff canonical-drift host-smoke release-parity publish-check

build:
	go build ./...

test:
	go test ./... -count=1

vet:
	go vet ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || \
	(command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "no linter installed, skipping")

check: build vet test

coverage:
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out

contract-snapshot:
	go run ./cmd/dotfiles-mcp-contract --write

contract-check:
	go run ./cmd/dotfiles-mcp-contract --check

contract-diff:
	bash ./scripts/contract-diff-summary.sh

canonical-drift:
	bash ./scripts/canonical-drift.sh

host-smoke:
	bash ./scripts/host-smoke.sh

release-parity:
	bash ./scripts/release-parity.sh

publish-check: vet test contract-check release-parity

HG_PIPELINE_MK ?= $(or $(wildcard $(abspath $(CURDIR)/../dotfiles/make/pipeline.mk)),$(wildcard $(HOME)/hairglasses-studio/dotfiles/make/pipeline.mk))
-include $(HG_PIPELINE_MK)
