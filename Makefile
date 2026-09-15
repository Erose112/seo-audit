BINARY   := seo-audit
MODULE   := github.com/Erose112/seo-audit
VERSION  ?= $(shell git describe --tags --always --dirty)
RAW      := $(patsubst v%,%,$(VERSION))
COMMIT   := $(shell git rev-parse --short HEAD)
DATE     := $(shell git log -1 --format=%cI)
LDFLAGS  := -s -w \
	-X $(MODULE)/internal/buildinfo.version=$(VERSION) \
	-X $(MODULE)/internal/buildinfo.commit=$(COMMIT) \
	-X $(MODULE)/internal/buildinfo.date=$(DATE)
PLATFORMS := linux/amd64 linux/arm64 windows/amd64
GOFLAGS  := -trimpath
export CGO_ENABLED := 0

# Host binary extension (.exe on Windows).
ifeq ($(OS),Windows_NT)
  BIN_EXT := .exe
  MKDIR   := mkdir
  RMRF    := rm -rf
else
  BIN_EXT :=
  MKDIR   := mkdir -p
  RMRF    := rm -rf
endif
HOST_BIN := bin/$(BINARY)$(BIN_EXT)

URL ?= https://example.com
CI_SIM_DIR := .ci-sim
BASELINE := $(CI_SIM_DIR)/latest.json

.PHONY: build test test-integration vet fmt-check dist ci-sim clean help

help:
	@echo "Targets:"
	@echo "  build              Build host binary to bin/"
	@echo "  test               Run unit tests"
	@echo "  test-integration   Run subprocess integration tests"
	@echo "  vet                Run go vet"
	@echo "  fmt-check          Fail if gofmt would change files"
	@echo "  dist               Cross-compile and archive all platforms to dist/"
	@echo "  ci-sim             Two-run baseline simulation (URL=$(URL))"
	@echo "  clean              Remove build artifacts"

build:
	@$(MKDIR) bin
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(HOST_BIN) .

test:
	go test ./...

test-integration:
	go test -tags integration ./cmd/ -count=1 -timeout 5m

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

dist:
	@$(MKDIR) dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		staging=dist/$(BINARY)_$(RAW)_$${os}_$${arch}; \
		$(MKDIR) $$staging; \
		ext=$$( [ "$$os" = windows ] && echo .exe ); \
		GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
			-o $$staging/$(BINARY)$$ext . ; \
		tar -czf $$staging.tar.gz -C dist $(BINARY)_$(RAW)_$${os}_$${arch}; \
		$(RMRF) $$staging; \
	done

ci-sim: build
	@$(MKDIR) $(CI_SIM_DIR)
	@echo "=== ci-sim run 1 (seed baseline) ==="
	$(HOST_BIN) crawl \
		--url "$(URL)" \
		--output json \
		--fail-below 80 \
		--baseline "$(BASELINE)" \
		--max-pages 50 \
		--max-duration 5m
	@test -f "$(BASELINE)" || (echo "baseline not written" && exit 1)
	@echo "=== ci-sim run 2 (compare against run 1) ==="
	$(HOST_BIN) crawl \
		--url "$(URL)" \
		--output json \
		--fail-below 80 \
		--baseline "$(BASELINE)" \
		--max-pages 50 \
		--max-duration 5m
	@echo "=== ci-sim complete ==="

clean:
	$(RMRF) bin dist $(CI_SIM_DIR)
