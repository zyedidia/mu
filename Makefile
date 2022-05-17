DETECT = buffer/ftdetect/detectors.dat
DETECT_SRC = $(wildcard buffer/ftdetect/*.json) buffer/ftdetect/generate.go
LDFLAGS = -ldflags "-s -w $(LINKVARS)"
TAGS = -tags flare_custom,ftdetect_custom
GOFLAGS = -trimpath $(LDFLAGS) $(TAGS)

all: build

install: $(DETECT)
	go install $(GOFLAGS) ./cmd/vned

build: $(DETECT)
	go build $(GOFLAGS) ./cmd/vned

test:
	go test ./...

cover.out:
	go test ./... -coverprofile cover.out

cover:
	go test ./... -cover

cover-total: cover.out
	go tool cover -func cover.out

$(DETECT): $(DETECT_SRC)
	go run buffer/ftdetect/generate.go -in $(dir $@) -out $@

.PHONY: cover cover-total test build
