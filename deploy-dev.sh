#!/bin/bash

# 0. Kill any hanging processes occupying port 8085 and 8081
echo "Cleaning up previous port-forwards..."
fuser -k 8085/tcp || true
fuser -k 8081/tcp || true
killall -9 kubectl || true

# 1. Build the images
./build-images.sh

# 2. Load the images into the local Kind cluster
echo "Loading images into Kind..."
kind load docker-image kairos-bootstrap:local --name kairos-vault
kind load docker-image kairos-node:local --name kairos-vault
kind load docker-image kairos-explorer:local --name kairos-vault

# 3. Install the P2P infrastructure with Helm
echo "Installing Helm Chart..."
helm upgrade --install kairos-engine ./helm \
  --set bootstrap.image=kairos-bootstrap:local \
  --set nodes.image=kairos-node:local \
  --set explorer.image=kairos-explorer:local \
  --set bootstrap.pullPolicy=Never \
  --set nodes.pullPolicy=Never \
  --set explorer.pullPolicy=Never

# 4. Wait for Kubernetes to finish the job
echo "Waiting for all pods to be ready (this may take a few seconds)..."
kubectl rollout status statefulset/kairos-bootstrap --timeout=120s
kubectl rollout status statefulset/kairos-node --timeout=120s
kubectl rollout status deployment/kairos-explorer --timeout=120s

# 5. Configure Port Forwards (in background)
echo "Configuring Node API Port-Forward on localhost:8085..."
kubectl port-forward svc/kairos-engine-api 8085:80 > /dev/null 2>&1 &

echo "Configuring Explorer Port-Forward on localhost:8081..."
kubectl port-forward svc/kairos-explorer 8081:8081 > /dev/null 2>&1 &

echo "Deploy completed! The P2P network is running."
echo "- Node API is on http://localhost:8085"
echo "- Explorer is on http://localhost:8081/network/overview"