#!/bin/bash

set -euf -o pipefail

docker pull golang:latest
docker pull alpine:latest
docker compose -f docker-compose.local.yml pull
docker compose -f docker-compose.local.yml stop
docker compose -f docker-compose.local.yml rm -f -v -s
docker compose -f docker-compose.local.yml up -d --remove-orphans
docker image prune -f
