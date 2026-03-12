# version.mk — Shared semver version resolution
#
# VERSION can be set explicitly (e.g. make VERSION=1.2.3) or is
# derived from the latest git tag with patch auto-incremented.
# Supports full semver: 1.2.3-rc1+build123
#
# Exports:
#   SEMVER_MAJOR, SEMVER_MINOR, SEMVER_PATCH
#   SEMVER_PRERELEASE, SEMVER_BUILDMETA, SEMVER_VERSION
#   BUILD_DATE, BUILD_VCS_URL, BUILD_VCS_ID, BUILD_VCS_ID_DATE

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

SEMVER_VERSION := $(SEMVER_MAJOR).$(SEMVER_MINOR).$(SEMVER_PATCH)
SEMVER_VERSION := $(SEMVER_VERSION)$(if $(SEMVER_PRERELEASE),-$(SEMVER_PRERELEASE))

BUILD_DATE        := $(shell date -u -Iseconds)
BUILD_VCS_URL     := $(shell git config --get remote.origin.url 2>/dev/null)
BUILD_VCS_ID      := $(shell git log -n 1 --date=iso-strict-local --format="%h" 2>/dev/null)
BUILD_VCS_ID_DATE := $(shell TZ=UTC0 git log -n 1 --date=iso-strict-local --format='%ad' 2>/dev/null)
