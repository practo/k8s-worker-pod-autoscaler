#!/bin/bash

set -e

echo "Generating Helm values..."
args=()
args+=(--set awsAccessKeyId="${AWS_ACCESS_KEY_ID}")
args+=(--set awsSecretAccessKey="${AWS_SECRET_ACCESS_KEY}")

readarray -d ',' -t regions <<<"${AWS_REGIONS}"
regions_json="$(printf '[%s]' $(printf '"%s",' ${regions[@]} | sed s'/,$//'))"
args+=(--set-json awsRegions="$regions_json")

echo "Image to be used: public.ecr.aws/practo/${WPA_TAG}"

echo "Deploying helm chart..."
helm install ../helm-charts/worker-pod-autoscaler "${args[@]}"
kubectl get pods -n kube-system | grep workerpodautoscaler
