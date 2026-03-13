NAME := shout

LISP     := sbcl
LISPOPTS := --no-sysinit --no-userinit
LISPEXEC := $(LISP) --script

BUILD := build
QLDIR   := $(BUILD)/quicklisp
QLURL   := http://beta.quicklisp.org/quicklisp.lisp
QLFILE  := $(QLDIR)/quicklisp.lisp
QLSETUP := $(QLDIR)/setup.lisp

LISP_QL := $(LISP) $(LISPOPTS) --load $(QLSETUP)

include ../version.mk

# Coverage settings (inherited from root Makefile or overridden on command line)
COVERAGE_MIN    ?= 50
COVERAGE_REPORT ?=

# dev builds get -dev prerelease tag
build: SEMVER_PRERELEASE := $(or $(SEMVER_PRERELEASE),dev)

# ── Targets ──────────────────────────────────────────────────────

default: build

# Use vendored quicklisp if available, otherwise download
$(QLDIR)/setup.lisp:
	@if [ -d vendor/quicklisp ]; then \
		echo "Using vendored Quicklisp dependencies"; \
		mkdir -p $(BUILD); \
		cp -r vendor/quicklisp $(BUILD)/quicklisp; \
	else \
		echo "Install Quicklisp (downloading)"; \
		mkdir -p $(QLDIR); \
		curl -o $(QLFILE) $(QLURL); \
		$(LISP) $(LISPOPTS) --load $(QLFILE) \
		  --eval '(quicklisp-quickstart:install :path "$(QLDIR)")' \
		  --quit; \
		rm $(QLFILE); \
	fi

quicklisp: $(QLDIR)/setup.lisp ;

$(BUILD)/.reqs: $(QLDIR)/setup.lisp
	mkdir -p $(BUILD)
	@if [ -d vendor/quicklisp ]; then \
		echo "Dependencies already vendored"; \
	else \
		echo "Downloading requirements"; \
		$(LISP_QL) --eval '(ql:quickload :hunchentoot)'  \
		           --eval '(ql:quickload :drakma)'       \
		           --eval '(ql:quickload :cl-json)'      \
		           --eval '(ql:quickload :daemon)'       \
		           --eval '(ql:quickload :prove)'        \
		           --eval '(load "$(NAME).asd")' --quit; \
	fi
	touch $@

libs: $(BUILD)/.reqs ;

version.lisp: FORCE
	@echo '(in-package :api)' > $@
	@echo '(defvar *semver-major* "$(SEMVER_MAJOR)")' >> $@
	@echo '(defvar *semver-minor* "$(SEMVER_MINOR)")' >> $@
	@echo '(defvar *semver-patch* "$(SEMVER_PATCH)")' >> $@
	@echo '(defvar *semver-prerelease* "$(SEMVER_PRERELEASE)")' >> $@
	@echo '(defvar *release-version* "$(SEMVER_VERSION)")' >> $@
	@echo '(defvar *release-name* "Whisper")' >> $@
	@echo '(defvar *build-date* "$(BUILD_DATE)")' >> $@
	@echo '(defvar *build-vcs-id* "$(BUILD_VCS_ID)")' >> $@
FORCE:

build: quicklisp libs version.lisp
	$(LISPEXEC) compile.lisp

test: quicklisp libs version.lisp
	rm -rf $(HOME)/.cache/common-lisp/
	$(LISPEXEC) test.lisp

coverage: quicklisp libs version.lisp
	@$(LISPEXEC) cover.lisp
ifeq ($(COVERAGE_REPORT),full)
	@echo "HTML report: coverage/"
else
	@TOTAL=$$(cat coverage/percent.txt 2>/dev/null || echo "0"); \
	echo "Total coverage: $${TOTAL}%"; \
	if [ $(COVERAGE_MIN) -gt 0 ] && [ $$(echo "$${TOTAL} < $(COVERAGE_MIN)" | bc) -eq 1 ]; then \
		echo "FAIL: coverage $${TOTAL}% is below $(COVERAGE_MIN)% threshold"; \
		exit 1; \
	else \
		echo "OK: coverage meets $(COVERAGE_MIN)% threshold"; \
	fi
endif

# Refresh vendored dependencies (run on connected machine)
vendor: quicklisp libs
	rm -rf vendor/quicklisp
	cp -r $(BUILD)/quicklisp vendor/quicklisp

docker:
	docker build \
	  --build-arg BUILD_DATE="$(shell date -u -Iminutes)" \
	  --build-arg VCS_REF="$(shell git rev-parse --short HEAD)" \
	  --platform linux/amd64 \
	  -t genesiscommunity/shout:ubuntu-jammy .

debug-version:
	@echo "SEMVER_MAJOR      = $(SEMVER_MAJOR)"
	@echo "SEMVER_MINOR      = $(SEMVER_MINOR)"
	@echo "SEMVER_PATCH      = $(SEMVER_PATCH)"
	@echo "SEMVER_PRERELEASE = $(SEMVER_PRERELEASE)"
	@echo "SEMVER_VERSION    = $(SEMVER_VERSION)"
	@echo "BUILD_DATE        = $(BUILD_DATE)"
	@echo "BUILD_VCS_ID      = $(BUILD_VCS_ID)"
	@echo "BUILD_VCS_URL     = $(BUILD_VCS_URL)"

# Release build (no -dev prerelease tag)
release: quicklisp libs version.lisp
	$(LISPEXEC) compile.lisp
	@echo "Lisp release binary built:"
	@ls -lh $(NAME)

# ── Security & Lint ──────────────────────────────────────────────

check:
	@SBLINT=$$(command -v sblint 2>/dev/null || echo $$HOME/.roswell/bin/sblint); \
	if [ -x "$$SBLINT" ]; then \
		echo "Running sblint..."; \
		CL_SOURCE_REGISTRY="$$(pwd)/build/quicklisp/dists/quicklisp/software//:$$(pwd)//" \
			"$$SBLINT" shout.asd; \
	else \
		echo "sblint not installed (install: ros install cxxxr/sblint)"; \
		exit 1; \
	fi

lint: check

trivy:
	trivy fs --scanners vuln,secret,misconfig .

trivy-image:
	trivy image genesiscommunity/shout:ubuntu-jammy

security:
	@SBLINT=$$(command -v sblint 2>/dev/null || echo $$HOME/.roswell/bin/sblint); \
	if [ -x "$$SBLINT" ]; then \
		echo "Running sblint..."; \
		CL_SOURCE_REGISTRY="$$(pwd)/build/quicklisp/dists/quicklisp/software//:$$(pwd)//" \
			"$$SBLINT" shout.asd; \
	else \
		echo "sblint not installed, skipping (install: ros install cxxxr/sblint)"; \
	fi
	trivy fs --scanners vuln,secret,misconfig .
	@echo ""
	@echo "Note: 'make check' runs sblint independently (fails if not installed)"

clean:
	rm -rf $(BUILD) version.lisp

.PHONY: default build test check lint clean vendor docker debug-version coverage release trivy trivy-image security FORCE
