# Flywheel Makefile — Go 工程标准目标（对齐 lark-cli 的 make 约定）。
SHELL := /bin/bash
BINARY := flywheel
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DATE := $(shell date +%Y-%m-%d)
LDFLAGS := -s -w -X github.com/ckanner/flywheel/internal/build.Version=$(VERSION) -X github.com/ckanner/flywheel/internal/build.Date=$(DATE)
# Hundreds of real Git/state scenarios share one package deadline. macOS CI
# needs more than Go's default 10m; individual test guards still apply.
FLYWHEEL_TEST_TIMEOUT ?= 15m

.PHONY: build vet fmt-check test coverage quality-gate clean install

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/flywheel

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l . | grep -v '^$$')" || (echo "gofmt needed:"; gofmt -l .; exit 1)

test:
	go test ./... -count=1 -timeout=$(FLYWHEEL_TEST_TIMEOUT)

# 覆盖率 100% 门禁（用户确立）：单元测试覆盖率反向驱动可测性。
# cmd/flywheel 的 main() 是单行 os.Exit 桩，无逻辑，不计入门禁。
# v0.18 收紧：AGENTS.md 契约是 per-package 100%，只看总数会放行单包缺口
# （cli 曾以 99.7% 混过）——逐函数检查，任何 <100% 直接失败。
coverage:
	go test $(shell go list ./... | grep -v /cmd/flywheel) -count=1 -timeout=$(FLYWHEEL_TEST_TIMEOUT) -coverprofile=cover.out
	@go tool cover -func=cover.out | tail -1
	@bad=$$(go tool cover -func=cover.out | awk '$$NF != "100.0%" && $$NF != "100%" {print}'); \
	if [ -n "$$bad" ]; then \
	  echo "FAIL: per-function coverage <100% (AGENTS.md Coverage Discipline):"; \
	  echo "$$bad"; exit 1; \
	fi
	@echo "OK: coverage 100% (every function)"

quality-gate: vet fmt-check coverage

install: build
	install -m 0755 $(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -rf $(BINARY) cover.out dist bin
