#!/bin/bash

set -e

if [ $# -eq 0 ]; then
  export WPA_TAG=`curl -s https://api.github.com/repos/practo/k8s-worker-pod-autoscaler/releases/latest|python -c "import json;import sys;sys.stdout.write(json.load(sys.stdin)['tag_name']+'\n')"`
else
  export WPA_TAG=$1
fi

crd='./artifacts/crd.yaml'
serviceaccount='./artifacts/serviceaccount.yaml'
clusterrole='./artifacts/clusterrole.yaml'
clusterrolebinding='./artifacts/clusterrolebinding.yaml'
new_deployment='./artifacts/deployment.yaml'
template_deployment='./artifacts/deployment-template.yaml'

echo "Creating CRD..."
kubectl apply -f ${crd}

echo "Generating Deployment Manifest..."
export WPA_AWS_REGIONS="${AWS_REGIONS}"
export WPA_AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}"
export WPA_AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}"

echo "Image to be used: practodev/${WPA_TAG}"

cp -f $template_deployment $new_deployment
./hack/generate.sh ${new_deployment}

echo "Applying manifests.."
kubectl apply -f ${serviceaccount}
kubectl apply -f ${clusterrole}
kubectl apply -f ${clusterrolebinding}
kubectl apply -f ${new_deployment}
kubectl get pods -n kube-system | grep workerpodautoscaler
