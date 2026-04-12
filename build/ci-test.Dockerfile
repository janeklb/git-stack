FROM golang:1.22.12-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends git jq make ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
