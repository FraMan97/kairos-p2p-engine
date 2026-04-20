#!/bin/bash

# 0. Kill any hanging processes occupying port 8080
echo "Cleaning up previous port-forwards..."
fuser -k 8080/tcp || true
killall kubectl || true

# 1. Build the images
./build-images.sh

# 2. Load the images into the local Kind cluster
echo "Loading images into Kind..."
kind load docker-image kairos-bootstrap:local --name kairos-vault
kind load docker-image kairos-node:local --name kairos-vault

# 3. Install the P2P infrastructure with Helm
echo "Installing Helm Chart..."
helm upgrade --install kairos-engine ./helm \
  --set bootstrap.image=kairos-bootstrap:local \
  --set nodes.image=kairos-node:local \
  --set bootstrap.pullPolicy=Never \
  --set nodes.pullPolicy=Never

# --- THE NEW PART: WAIT FOR KUBERNETES TO FINISH THE JOB ---
echo "Waiting for Bootstrap and Node pods to be ready (this may take a few seconds)..."
kubectl rollout status statefulset/kairos-bootstrap --timeout=120s
kubectl rollout status statefulset/kairos-node --timeout=120s

# 4. Configure Port Forward (in background) to test with the CLI
echo "Configuring Port-Forward on localhost:8080..."
kubectl port-forward svc/kairos-engine-api 8080:80 &

echo "Deploy completed! You can use k-cli to test the engine on localhost:8080"