#!/bin/bash

set -euf -o pipefail

docker pull golang:latest
docker pull alpine:latest
docker pull cloudflare/cloudflared:latest
docker compose -f docker-compose.cloudflare.yml pull
docker compose -f docker-compose.cloudflare.yml build
docker compose -f docker-compose.cloudflare.yml stop
docker compose -f docker-compose.cloudflare.yml rm -f -v -s
docker compose -f docker-compose.cloudflare.yml up -d --remove-orphans
docker image prune -f
