NAME   := shout
MODULE := github.com/cloudfoundry-community/shout
SHELL  := /bin/bash

include version.mk

# ── Platform selection ──────────────────────────────────────────
# Usage:
#   make build          — build both Go and Lisp
#   make build go       — build Go only
#   make build lisp     — build Lisp only
#   make test go        — test Go only
#   (same pattern for: test, clean, coverage, docker)

WANT_GO   :=
WANT_LISP :=

ifneq ($(filter go,$(MAKECMDGOALS)),)
  WANT_GO := yes
endif
ifneq ($(filter lisp,$(MAKECMDGOALS)),)
  WANT_LISP := yes
endif

# Default: both when neither specified
ifeq ($(WANT_GO)$(WANT_LISP),)
  WANT_GO   := yes
  WANT_LISP := yes
endif

GO_TARGETS   = $(if $(WANT_GO),$1)
LISP_TARGETS = $(if $(WANT_LISP),$1)

# No-op targets so "make build go" doesn't error on target "go"
.PHONY: go lisp
go lisp:
	@:

# ── Common targets ─────────────────────────────────────────────

.PHONY: build test clean coverage docker

build: $(call GO_TARGETS,go-build) $(call LISP_TARGETS,lisp-build)
test:  $(call GO_TARGETS,go-test)  $(call LISP_TARGETS,lisp-test)
clean: $(call GO_TARGETS,go-clean) $(call LISP_TARGETS,lisp-clean)
coverage: $(call GO_TARGETS,go-coverage) $(call LISP_TARGETS,lisp-coverage)
docker: $(call GO_TARGETS,go-docker) $(call LISP_TARGETS,lisp-docker)

# ── Go targets ─────────────────────────────────────────────────

VERPKG := $(MODULE)/pkg/version

GO_LDFLAGS = -s -w \
	-X '$(VERPKG).SemVerMajor=$(SEMVER_MAJOR)' \
	-X '$(VERPKG).SemVerMinor=$(SEMVER_MINOR)' \
	-X '$(VERPKG).SemVerPatch=$(SEMVER_PATCH)' \
	-X '$(VERPKG).SemVerPrerelease=$(SEMVER_PRERELEASE)' \
	-X '$(VERPKG).SemVerBuild=$(SEMVER_BUILDMETA)' \
	-X '$(VERPKG).BuildDate=$(BUILD_DATE)' \
	-X '$(VERPKG).BuildVcsUrl=$(BUILD_VCS_URL)' \
	-X '$(VERPKG).BuildVcsId=$(BUILD_VCS_ID)' \
	-X '$(VERPKG).BuildVcsIdDate=$(BUILD_VCS_ID_DATE)'

.PHONY: go-build go-test go-check go-clean go-coverage go-docker

# dev builds get -dev prerelease tag
go-build: SEMVER_PRERELEASE := $(or $(SEMVER_PRERELEASE),dev)
go-build:
	go build -ldflags="$(GO_LDFLAGS)" -o $(NAME) ./cmd/shout

go-test: go-check
	go test -race ./cmd/... ./internal/... ./pkg/...

go-check:
	go fmt ./...
	go vet ./...

go-coverage:
	go test -coverprofile=coverage.out ./cmd/... ./internal/... ./pkg/...
	go tool cover -func=coverage.out

go-clean:
	rm -f $(NAME) $(NAME)-linux-* $(NAME)-darwin-* coverage.out coverage.html

go-docker:
	docker build -t $(NAME):latest .

# ── Go-only targets ────────────────────────────────────────────

.PHONY: fmt vet check coverage-html coverage-check
.PHONY: gosec govulncheck trivy trivy-image security
.PHONY: linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 all-platforms

fmt:
	go fmt ./...

vet:
	go vet ./...

check: fmt vet

coverage-html: go-coverage
	go tool cover -html=coverage.out -o coverage.html

coverage-check:
	@go test -coverprofile=coverage.out ./cmd/... ./internal/... ./pkg/... > /dev/null 2>&1
	@TOTAL=$$(go tool cover -func=coverage.out | grep ^total: | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${TOTAL}%"; \
	if [ $$(echo "$${TOTAL} < 50" | bc) -eq 1 ]; then \
		echo "FAIL: coverage $${TOTAL}% is below 50% threshold"; \
		exit 1; \
	else \
		echo "OK: coverage meets 50% threshold"; \
	fi

gosec:
	gosec ./...

govulncheck:
	govulncheck ./...

trivy:
	trivy fs --scanners vuln,secret,misconfig .

trivy-image:
	trivy image $(NAME):latest

security: gosec govulncheck trivy

linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS)" -o $(NAME)-linux-amd64 ./cmd/shout

linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="$(GO_LDFLAGS)" -o $(NAME)-linux-arm64 ./cmd/shout

darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS)" -o $(NAME)-darwin-amd64 ./cmd/shout

darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(GO_LDFLAGS)" -o $(NAME)-darwin-arm64 ./cmd/shout

all-platforms: linux-amd64 linux-arm64 darwin-amd64 darwin-arm64

# ── Lisp targets (delegate to lisp/Makefile) ───────────────────

.PHONY: lisp-build lisp-test lisp-clean lisp-coverage lisp-docker

lisp-build:
	@$(MAKE) --no-print-directory -C lisp build

lisp-test:
	@$(MAKE) --no-print-directory -C lisp test

lisp-clean:
	@$(MAKE) --no-print-directory -C lisp clean

lisp-coverage:
	@$(MAKE) --no-print-directory -C lisp coverage

lisp-docker:
	@$(MAKE) --no-print-directory -C lisp docker

# ── Version ────────────────────────────────────────────────────

.PHONY: debug-version
debug-version:
	@echo "SEMVER_VERSION    $(SEMVER_VERSION)"
	@echo "SEMVER_MAJOR      $(SEMVER_MAJOR)"
	@echo "SEMVER_MINOR      $(SEMVER_MINOR)"
	@echo "SEMVER_PATCH      $(SEMVER_PATCH)"
	@echo "SEMVER_PRERELEASE $(SEMVER_PRERELEASE)"
	@echo "SEMVER_BUILDMETA  $(SEMVER_BUILDMETA)"
	@echo "BUILD_DATE        $(BUILD_DATE)"
	@echo "BUILD_VCS_URL     $(BUILD_VCS_URL)"
	@echo "BUILD_VCS_ID      $(BUILD_VCS_ID)"
	@echo "BUILD_VCS_ID_DATE $(BUILD_VCS_ID_DATE)"

# ── Help ───────────────────────────────────────────────────────

.PHONY: help
help:
	@echo "Usage: make <target> [go|lisp]"
	@echo ""
	@echo "Common (supports platform selector):"
	@echo "  build            Build binary (default: both)"
	@echo "  test             Run tests (default: both)"
	@echo "  clean            Remove build artifacts (default: both)"
	@echo "  coverage         Generate coverage report (default: both)"
	@echo "  docker           Build Docker image (default: both)"
	@echo ""
	@echo "Examples:"
	@echo "  make build         Build both Go and Lisp"
	@echo "  make build go      Build Go only"
	@echo "  make test lisp     Test Lisp only"
	@echo ""
	@echo "Go only:"
	@echo "  fmt              Run go fmt"
	@echo "  vet              Run go vet"
	@echo "  check            Run fmt + vet"
	@echo "  coverage-html    Generate HTML coverage report"
	@echo "  coverage-check   Verify coverage meets 50%% threshold"
	@echo "  security         Run gosec + govulncheck + trivy"
	@echo "  all-platforms    Cross-compile all platform variants"
	@echo ""
	@echo "Version:"
	@echo "  debug-version    Print resolved version variables"
	@echo "  Override: make VERSION=1.2.3 build"
