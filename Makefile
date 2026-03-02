NAME := shout

LISP     := sbcl
LISPOPTS := --no-sysinit --no-userinit
LISPEXEC := $(LISP) --script

BUILD := build
BUILDAPP = $(BUILD)/buildapp

QLDIR   := $(BUILD)/quicklisp
QLURL   := http://beta.quicklisp.org/quicklisp.lisp
QLFILE  := $(QLDIR)/quicklisp.lisp
QLSETUP := $(QLDIR)/setup.lisp

LISP_QL := $(LISP) $(LISPOPTS) --load $(QLSETUP)

PREFIX = /usr/local
INSTALL_DIR = $(DESTDIR)$(PREFIX)/bin

# ── Version ──────────────────────────────────────────────────────
# VERSION can be set explicitly (e.g. make VERSION=1.2.3) or is
# derived from the latest git tag with patch auto-incremented.
# Supports full semver: 1.2.3-rc1+build123

define is_not_number
$(shell echo ${1} | sed -e 's/[0123456789]//g')
endef

CLEAN_VERSION       = $(patsubst v%,%,$(VERSION))
HAS_BUILDMETA      := $(findstring +,$(CLEAN_VERSION))
VERSION_BUILDMETA   := $(if $(HAS_BUILDMETA),$(lastword $(subst +, ,$(CLEAN_VERSION))),)

VERSION_AND_PRERELEASE := $(firstword $(subst +, ,$(CLEAN_VERSION)))

HAS_PRERELEASE      := $(findstring -,$(VERSION_AND_PRERELEASE))
VERSION_ONLY        := $(firstword $(subst -, ,$(VERSION_AND_PRERELEASE)))
VERSION_PRERELEASE  := $(if $(HAS_PRERELEASE),$(patsubst $(VERSION_ONLY)-%,%,$(VERSION_AND_PRERELEASE)),)

ifneq ($(VERSION_ONLY),)
VERSION_SPLIT := $(subst ., ,$(VERSION_ONLY))
  ifneq ($(words $(VERSION_SPLIT)),3)
    $(error VERSION does not have 3 parts: $(VERSION))
  endif
else
VERSION_TAG         := $(shell git describe --tags --abbrev=0 2>/dev/null || echo 0.0.0)
CLEAN_VERSION_TAG    = $(patsubst v%,%,$(VERSION_TAG))
VERSION_SPLIT       := $(subst ., ,$(CLEAN_VERSION_TAG))
  ifneq ($(words $(VERSION_SPLIT)),3)
    $(error VERSION_TAG does not have 3 parts: $(VERSION_TAG))
  endif
  ifneq ($(words $(call is_not_number,$(word 3,$(VERSION_SPLIT)))), 0)
    $(error VERSION_TAG patch contains non-numeric characters: $(VERSION_TAG))
  endif
  # Auto-increment patch
  VERSION_SPLIT := $(wordlist 1, 2, $(VERSION_SPLIT)) $(shell echo $$(($(word 3,$(VERSION_SPLIT))+1)))
endif

ifneq ($(words $(call is_not_number,$(VERSION_SPLIT))), 0)
  $(error Version contains non-numeric characters: $(VERSION_SPLIT))
endif

SEMVER_MAJOR      ?= $(word 1,$(VERSION_SPLIT))
SEMVER_MINOR      ?= $(word 2,$(VERSION_SPLIT))
SEMVER_PATCH      ?= $(word 3,$(VERSION_SPLIT))
SEMVER_PRERELEASE ?= $(VERSION_PRERELEASE)
SEMVER_BUILDMETA  ?= $(VERSION_BUILDMETA)

BUILD_DATE        := $(shell date -u -Iseconds)
BUILD_VCS_URL     := $(shell git config --get remote.origin.url 2>/dev/null)
BUILD_VCS_ID      := $(shell git log -n 1 --date=iso-strict-local --format="%h" 2>/dev/null)
BUILD_VCS_ID_DATE := $(shell TZ=UTC0 git log -n 1 --date=iso-strict-local --format='%ad' 2>/dev/null)

# dev builds get -dev prerelease tag
shout: SEMVER_PRERELEASE := $(or $(SEMVER_PRERELEASE),dev)

SEMVER_VERSION := $(SEMVER_MAJOR).$(SEMVER_MINOR).$(SEMVER_PATCH)
SEMVER_VERSION := $(SEMVER_VERSION)$(if $(SEMVER_PRERELEASE),-$(SEMVER_PRERELEASE))

# ── Targets ──────────────────────────────────────────────────────

default: all

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

all: shout
shout: quicklisp libs version.lisp
	$(LISPEXEC) compile.lisp

test: quicklisp libs
	rm -rf $(HOME)/.cache/common-lisp/
	$(LISPEXEC) test.lisp
coverage: quicklisp libs
	$(LISPEXEC) cover.lisp

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

clean:
	rm -rf $(BUILD) version.lisp

.PHONY: all test clean vendor docker debug-version FORCE
