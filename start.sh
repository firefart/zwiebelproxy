#!/bin/bash

set -euf -o pipefail

docker pull golang:latest
docker pull alpine:latest
docker compose pull
docker compose build
docker compose stop
docker compose rm -f -v -s
docker compose up -d --remove-orphans
docker image prune -f
