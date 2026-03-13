NAME   := shout
MODULE := github.com/cloudfoundry-community/shout
SHELL  := /bin/bash

# Coverage settings
#   COVERAGE_MIN    — gate threshold (default 50, set 0 to disable gate)
#   COVERAGE_REPORT — report type: full (HTML report), default is gate
COVERAGE_MIN    ?= 50
COVERAGE_REPORT ?=

include version.mk

# ── Platform selection ──────────────────────────────────────────
# Usage:
#   make build          — build both Go and Lisp
#   make build go       — build Go only
#   make build lisp     — build Lisp only
#   make test go        — test Go only
#   (same pattern for: test, clean, coverage, docker, release,
#    security, help)

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

.PHONY: build test check clean coverage docker release security help

build:    $(call GO_TARGETS,go-build)    $(call LISP_TARGETS,lisp-build)
test:     $(call GO_TARGETS,go-test)     $(call LISP_TARGETS,lisp-test)
check:    $(call GO_TARGETS,go-check)    $(call LISP_TARGETS,lisp-check)
clean:    $(call GO_TARGETS,go-clean)    $(call LISP_TARGETS,lisp-clean)
coverage: $(call GO_TARGETS,go-coverage) $(call LISP_TARGETS,lisp-coverage)
docker:   $(call GO_TARGETS,go-docker)   $(call LISP_TARGETS,lisp-docker)
release:  $(call GO_TARGETS,go-release)  $(call LISP_TARGETS,lisp-release)
security: $(call GO_TARGETS,go-security) $(call LISP_TARGETS,lisp-security)
help: help-usage $(call GO_TARGETS,go-help) $(call LISP_TARGETS,lisp-help) help-variables

.PHONY: help-usage help-variables
help-usage:
	@echo "Usage: make <target> [go|lisp]"
	@echo ""
	@echo "  Append 'go' or 'lisp' to run for one implementation only."
	@echo "  Omit the selector to run for both."
	@echo ""

help-variables:
	@echo "Variables:"
	@echo "  COVERAGE_MIN=N       Coverage gate threshold (default: 50, 0 to disable)"
	@echo "  COVERAGE_REPORT=full Generate full HTML report instead of gate"
	@echo "  VERSION=x.y.z        Override semver version"
	@echo ""

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
.PHONY: go-release go-security go-help

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
	@go test -coverprofile=coverage.out ./cmd/... ./internal/... ./pkg/...
ifeq ($(COVERAGE_REPORT),full)
	@go tool cover -func=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "HTML report: coverage.html"
else
	@TOTAL=$$(go tool cover -func=coverage.out | grep ^total: | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${TOTAL}%"; \
	if [ $(COVERAGE_MIN) -gt 0 ] && [ $$(echo "$${TOTAL} < $(COVERAGE_MIN)" | bc) -eq 1 ]; then \
		echo "FAIL: coverage $${TOTAL}% is below $(COVERAGE_MIN)% threshold"; \
		exit 1; \
	else \
		echo "OK: coverage meets $(COVERAGE_MIN)% threshold"; \
	fi
endif

go-clean:
	rm -f $(NAME) $(NAME)-linux-* $(NAME)-darwin-* coverage.out coverage.html

go-docker:
	docker build -t $(NAME):latest .

go-release: go-release-linux-amd64 go-release-linux-arm64 go-release-darwin-amd64 go-release-darwin-arm64
	@echo "Go release binaries built:"
	@ls -lh $(NAME)-linux-* $(NAME)-darwin-* 2>/dev/null

go-security: go-security-gosec go-security-govulncheck go-security-trivy

go-help:
	@echo "Go targets:"
	@echo "  build            Build binary for current platform"
	@echo "  test             Run fmt + vet + tests with race detection"
	@echo "  check            Run go fmt + go vet"
	@echo "  clean            Remove build artifacts and coverage files"
	@echo "  coverage         Coverage gate at COVERAGE_MIN=$(COVERAGE_MIN)% (COVERAGE_REPORT=full for HTML)"
	@echo "  docker           Build Docker image"
	@echo "  release          Cross-compile all platform binaries"
	@echo "  security         Run gosec + govulncheck + trivy"
	@echo "  debug-version    Print resolved version variables"
	@echo ""

# ── Go release (cross-compilation) ────────────────────────────

.PHONY: go-release-linux-amd64 go-release-linux-arm64
.PHONY: go-release-darwin-amd64 go-release-darwin-arm64

go-release-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS)" -o $(NAME)-linux-amd64 ./cmd/shout

go-release-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="$(GO_LDFLAGS)" -o $(NAME)-linux-arm64 ./cmd/shout

go-release-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS)" -o $(NAME)-darwin-amd64 ./cmd/shout

go-release-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(GO_LDFLAGS)" -o $(NAME)-darwin-arm64 ./cmd/shout

# ── Go security scanning ─────────────────────────────────────

.PHONY: go-security-gosec go-security-govulncheck go-security-trivy

go-security-gosec:
	gosec ./...

go-security-govulncheck:
	govulncheck ./...

go-security-trivy:
	trivy fs --scanners vuln,secret,misconfig .

# ── Lisp targets (delegate to lisp/Makefile) ───────────────────

.PHONY: lisp-build lisp-test lisp-check lisp-clean lisp-coverage lisp-docker
.PHONY: lisp-release lisp-security lisp-help

lisp-build:
	@$(MAKE) --no-print-directory -C lisp build

lisp-test:
	@$(MAKE) --no-print-directory -C lisp test

lisp-check:
	@$(MAKE) --no-print-directory -C lisp check

lisp-clean:
	@$(MAKE) --no-print-directory -C lisp clean

lisp-coverage:
	@$(MAKE) --no-print-directory -C lisp coverage COVERAGE_MIN=$(COVERAGE_MIN) COVERAGE_REPORT=$(COVERAGE_REPORT)

lisp-docker:
	@$(MAKE) --no-print-directory -C lisp docker

lisp-release:
	@$(MAKE) --no-print-directory -C lisp release

lisp-security:
	@$(MAKE) --no-print-directory -C lisp security

lisp-help:
	@echo "Lisp targets:"
	@echo "  build            Build standalone executable"
	@echo "  test             Run test suite (prove framework)"
	@echo "  check            Run sblint static analysis"
	@echo "  clean            Remove build artifacts"
	@echo "  coverage         Coverage gate at COVERAGE_MIN=$(COVERAGE_MIN)% (COVERAGE_REPORT=full for HTML)"
	@echo "  docker           Build Docker image (linux/amd64)"
	@echo "  release          Build release executable"
	@echo "  security         Run sblint + trivy"
	@echo ""

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
