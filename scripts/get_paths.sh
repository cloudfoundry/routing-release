#!/bin/bash

set -x

absolute_path() {
  (cd $1 && pwd)
}

scripts_path=$(absolute_path `dirname $0`)

ROUTING_RELEASE_DIR=${ROUTING_RELEASE_DIR:-$(absolute_path $scripts_path/..)}

ROOT_DIR=$(
  for i in `seq 1 5`; do
    ls "cf-release" >> "/dev/null"
    if [[ $? -ne 0 ]]; then  >> "/dev/null"
      cd ..
    else
      break
    fi
  done
  pwd
)

echo $ROOT_DIR

DIEGO_RELEASE_DIR=${DIEGO_RELEASE_DIR:-$ROOT_DIR/diego-release}
CF_RELEASE_DIR=${CF_RELEASE_DIR:-$ROOT_DIR/cf-release}

ROUTING_MANIFESTS_DIR=$ROUTING_RELEASE_DIR/bosh-lite/deployments
CF_MANIFESTS_DIR=$CF_RELEASE_DIR/bosh-lite/deployments
DIEGO_MANIFESTS_DIR=$DIEGO_RELEASE_DIR/bosh-lite/deployments

echo ROUTING_RELEASE_DIR=$ROUTING_RELEASE_DIR
echo DIEGO_RELEASE_DIR=$DIEGO_RELEASE_DIR
echo CF_RELEASE_DIR=$CF_RELEASE_DIR

echo ROUTING_MANIFESTS_DIR=$ROUTING_MANIFESTS_DIR
echo CF_MANIFESTS_DIR=$CF_MANIFESTS_DIR
echo DIEGO_MANIFESTS_DIR=$DIEGO_MANIFESTS_DIR
