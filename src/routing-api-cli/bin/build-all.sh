#!/bin/sh

set -e
set -x

OUTDIR=$(dirname $0)/../out

version=${RTR_VERSION:-"BUILT_FROM_SOURCE"}
mkdir -p out

GOARCH=amd64 GOOS=linux go build -o rtr -ldflags "-X main.version=$version" && mv rtr $OUTDIR/rtr-linux-amd64
GOARCH=amd64 GOOS=darwin go build -o rtr -ldflags "-X main.version=$version" && mv rtr $OUTDIR/rtr-darwin-amd64
