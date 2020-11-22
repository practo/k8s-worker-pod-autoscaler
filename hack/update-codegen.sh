#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../code-generator)}

# TODO: this script works when there is a link in GOPATH to point to this repo, make it work without it (go modules)
bash "${CODEGEN_PKG}/generate-groups.sh" all \
     github.com/practo/k8s-worker-pod-autoscaler/pkg/generated \
     github.com/practo/k8s-worker-pod-autoscaler/pkg/apis \
     workerpodautoscaler:v1 \
     --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt"
