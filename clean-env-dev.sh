#!/bin/bash

echo "Removing Helm installation..."
helm uninstall kairos-engine

echo "Deleting PVCs to reset local BadgerDB databases..."
kubectl delete pvc -l app=kairos-bootstrap
kubectl delete pvc -l app=kairos-node

echo "Cleaning up orphan images in the Kind cluster..."
docker exec -it kairos-vault-control-plane crictl rmi --prune

echo "Development environment cleaned!"