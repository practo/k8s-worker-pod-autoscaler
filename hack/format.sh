#!/bin/sh
set -o errexit
set -o nounset
set -o pipefail

find . -type f | grep .go$ | grep -v vendor | xargs -I {} gofmt -w {}

