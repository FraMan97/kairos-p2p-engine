#!/bin/bash
echo "Deploying Kairos P2P Engine Alpha..."

helm upgrade --install kairos-engine ./helm \
  -f ./helm/value-dev.yaml \
  -f ./helm/values-prod.yaml \
  --namespace production --create-namespace

kubectl rollout restart statefulset/kairos-bootstrap -n production
kubectl rollout restart statefulset/kairos-node -n production
kubectl rollout restart deployment/kairos-explorer -n production

echo "Engine Deployment Completed."