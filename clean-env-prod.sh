helm uninstall kairos-engine -n production

kubectl delete pvc -l app=kairos-bootstrap -n production

kubectl delete pvc -l app=kairos-node -n production

./deploy-prod.sh