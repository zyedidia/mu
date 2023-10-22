#!/bin/sh

set -ex

VERSION=`cd tools/version; go run version.go`
DATE=`cd tools/date; go run date.go`
COMMIT=`git rev-parse --short HEAD`
go build -trimpath -ldflags "-s -w -X 'github.com/zyedidia/mu/build.Version=$VERSION' -X 'github.com/zyedidia/mu/build.CompileDate=$DATE' -X 'github.com/zyedidia/mu/build.CommitHash=$COMMIT'" -tags flare_custom,ftdetect_custom  ./cmd/mu
