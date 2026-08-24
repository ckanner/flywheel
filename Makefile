# Flywheel Makefile — Go 工程标准目标（对齐 lark-cli 的 make 约定）。
SHELL := /bin/bash
BINARY := flywheel
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DATE := $(shell date +%Y-%m-%d)
LDFLAGS := -s -w -X github.com/ckanner/flywheel/internal/build.Version=$(VERSION) -X github.com/ckanner/flywheel/internal/build.Date=$(DATE)

.PHONY: build vet fmt-check test coverage quality-gate clean install

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/flywheel

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l . | grep -v '^$$')" || (echo "gofmt needed:"; gofmt -l .; exit 1)

test:
	go test ./... -count=1

# 覆盖率 100% 门禁（用户确立）：单元测试覆盖率反向驱动可测性。
# cmd/flywheel 的 main() 是单行 os.Exit 桩，无逻辑，不计入门禁。
coverage:
	go test $(shell go list ./... | grep -v /cmd/flywheel) -count=1 -coverprofile=cover.out
	@go tool cover -func=cover.out | tail -1
	@total=$$(go tool cover -func=cover.out | tail -1 | awk '{print $$3}' | tr -d '%'); \
	if [ "$$(echo "$$total >= 100.0" | bc)" != "1" ]; then \
	  echo "FAIL: coverage $$total% < 100%"; exit 1; \
	fi
	@echo "OK: coverage 100%"

quality-gate: vet fmt-check coverage

install: build
	install -m 0755 $(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -rf $(BINARY) cover.out dist bin
