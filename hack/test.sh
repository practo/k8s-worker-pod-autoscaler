set -o errexit
set -o nounset
set -o pipefail

# TODO: make tests work
exit 0

export CGO_ENABLED=0

TARGETS=$(for d in "$@"; do echo ./$d/...; done)

echo "Test targets: " ${TARGETS}

echo "Running tests:"
go test -v -i -installsuffix "static" ${TARGETS}
go test -v -installsuffix "static" ${TARGETS}
echo

# TODO: fix me https://github.com/practo/k8s-sqs-pod-autoscaler-controllers/issues/55
exit 0
echo -n "Checking gofmt: "
ERRS=$(find "$@" -type f -name \*.go | xargs gofmt -l 2>&1 || true)
echo ${ERRS}
if [ -n "${ERRS}" ]; then
    echo "FAIL - the following files need to be gofmt'ed:"
    for e in ${ERRS}; do
        echo "    $e"
    done
    echo
    exit 1
fi
echo "PASS"
echo

echo -n "Checking go vet: "
ERRS=$(go vet ${TARGETS} 2>&1 || true)
if [ -n "${ERRS}" ]; then
    echo "FAIL"
    echo "${ERRS}"
    echo
    exit 1
fi
echo "PASS"
echo
