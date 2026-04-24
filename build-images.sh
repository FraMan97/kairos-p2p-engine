#!/bin/bash

echo "Building Bootstrap Server image..."
docker build -t kairos-bootstrap:local -f Dockerfile.bootstrap .

echo "Building P2P Node image..."
docker build -t kairos-node:local -f Dockerfile.node .

echo "Building CLI image..."
docker build -t kairos-cli:local -f Dockerfile.cli .

echo "Building Explorer image..."
docker build -t kairos-explorer:local -f Dockerfile.explorer .