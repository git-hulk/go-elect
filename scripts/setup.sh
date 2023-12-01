#!/usr/bin/env bash
set -e -x

cd scripts/docker && docker-compose up --force-recreate -d && cd ../..