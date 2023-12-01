#!/usr/bin/env bash
set -e -x
cd scripts/docker && docker-compose down --remove-orphans && cd ../..