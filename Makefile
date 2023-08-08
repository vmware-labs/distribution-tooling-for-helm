# Required for globs to work correctly
SHELL      = /usr/bin/env bash

BINDIR      := $(CURDIR)/bin
BINNAME     ?= dt

PROJECT_PLUGIN_SHORTNAME := helm-dt

GOPATH ?= $(shell go env GOPATH)
PATH := $(GOPATH)/bin:$(PATH)

BUILD_DIR := $(abspath ./out)

PKG         := github.com/vmware-labs/distribution-tooling-for-helm

VERSION := $(shell sed -n -e 's/version:[ "]*\([^"]*\).*/\1/p' plugin.yaml)


# Rebuild the binary if any of these files change
SRC := $(shell find . -type f -name '*.go' -print) go.mod go.sum

GOBIN         = $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN         = $(shell go env GOPATH)/bin
endif
GOIMPORTS     = $(GOBIN)/goimports
ARCH          = $(shell uname -p)

TAGS        :=
TESTS       := .
TESTFLAGS   :=
LDFLAGS     := -w -s
GOFLAGS     :=
CGO_ENABLED ?= 0

BUILD_DATE := $(shell date -u '+%Y-%m-%d %I:%M:%S UTC' 2> /dev/null)
GIT_HASH := $(shell git rev-parse HEAD 2> /dev/null)

LDFLAGS += -X "main.BuildDate=$(BUILD_DATE)"
LDFLAGS += -X main.Commit=$(GIT_HASH)

GO_MOD := @go mod


HELM_3_PLUGINS = $(shell helm env HELM_PLUGINS)
HELM_PLUGIN_DIR = $(HELM_3_PLUGINS)/$(PROJECT_PLUGIN_SHORTNAME)

.PHONY: all
all: build

# ------------------------------------------------------------------------------
#  build

.PHONY: build
build: $(BINDIR)/$(BINNAME)

$(BINDIR)/$(BINNAME): $(SRC)
	GO111MODULE=on CGO_ENABLED=$(CGO_ENABLED) go build $(GOFLAGS) -trimpath -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o '$(BINDIR)'/$(BINNAME) ./cmd/$(BINNAME)

# ------------------------------------------------------------------------------
#  install

.PHONY: install
install: build
	mkdir -p "$(HELM_PLUGIN_DIR)/bin"
	cp "$(BINDIR)/$(BINNAME)" "$(HELM_PLUGIN_DIR)/bin"
	cp plugin.yaml "$(HELM_PLUGIN_DIR)/"


# ------------------------------------------------------------------------------
#  test

.PHONY: test
test: build
test: test-style 
test: test-unit

.PHONY: test-unit
test-unit:
	@echo
	@echo "==> Running unit tests <=="
	GO111MODULE=on go test $(GOFLAGS) -run $(TESTS) ./... $(TESTFLAGS)

.PHONY: test-coverage
test-coverage:
	@echo
	@echo "==> Running unit tests with coverage <=="
	@mkdir -p $(BUILD_DIR)
	GO111MODULE=on go test -v -covermode=count -coverprofile=$(BUILD_DIR)/cover.out ./...
	GO111MODULE=on go tool cover -html=$(BUILD_DIR)/cover.out -o=$(BUILD_DIR)/coverage.html

.PHONY: test-style
test-style:
	GO111MODULE=on golangci-lint run

.PHONY: format
format: $(GOIMPORTS)
	GO111MODULE=on go list -f '{{.Dir}}' ./... | xargs $(GOIMPORTS) -w -local helm.sh/helm


# ------------------------------------------------------------------------------
#  dependencies

$(GOIMPORTS):
	(cd /; GO111MODULE=on go install golang.org/x/tools/cmd/goimports@latest)


# ------------------------------------------------------------------------------

.PHONY: clean
clean:
	@rm -rf '$(BINDIR)/$(BINNAME)'

