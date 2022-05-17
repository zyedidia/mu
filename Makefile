-include conf.mk

VERSION = $(shell go run ./tools/build-version.go)
DATE = $(shell go run ./tools/build-date.go)
COMMIT = $(shell git rev-parse --short HEAD)
BUILDPKG = github.com/zyedidia/ned/build

ifneq ($(FASTBUILD),1)
LINKVARS += -X '$(BUILDPKG).Version=$(VERSION)' -X '$(BUILDPKG).CompileDate=$(DATE)' -X '$(BUILDPKG).CommitHash=$(COMMIT)'
endif

ifeq ($(DEBUG),1)
LINKVARS += -X '$(BUILDPKG).Debug=ON'
endif

LDFLAGS = -ldflags "-s -w $(LINKVARS)"
TAGS = -tags flare_custom,ftdetect_custom
GOFLAGS = -trimpath $(LDFLAGS) $(TAGS)

all: build

install:
	go install $(GOFLAGS) ./cmd/vned

build:
	go build $(GOFLAGS) ./cmd/vned

ned:
	go build $(GOFLAGS) ./cmd/ned

test:
	go test ./...

cover.out:
	go test ./... -coverprofile cover.out

cover:
	go test ./... -cover

cover-total: cover.out
	go tool cover -func cover.out

.PHONY: cover cover-total test build ned install
