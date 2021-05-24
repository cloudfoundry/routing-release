#!/bin/sh

set -e
set -x

OUTDIR=$(dirname $0)/../out

$(dirname $0)/build-all.sh

pushd $OUTDIR
cp rtr-darwin-amd64 rtr && tar -zcf rtr-darwin-amd64.tgz rtr && rm rtr
cp rtr-linux-amd64 rtr && tar -zcf rtr-linux-amd64.tgz rtr && rm rtr
popd
