#!/usr/bin/env bash
set -e -x
kind_start() {
  kind create cluster --config kind/kind-cluster.yaml  --image kindest/node:v1.23.4 --name=test
  kind get kubeconfig --name=test > ../k8s.yaml 
}

kind_stop() {
    kind delete cluster -n test
    rm -rf ../k8s.yaml
}

cd scripts/docker && docker-compose up --force-recreate -d && cd ../..



