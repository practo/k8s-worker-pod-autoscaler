#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

export BIN='WPA'

unameOut="$(uname -s)"
case "${unameOut}" in
    Linux*)     machine=Linux;;
    Darwin*)    machine=Mac;;
    CYGWIN*)    machine=Cygwin;;
    MINGW*)     machine=MinGw;;
    *)          machine="UNKNOWN:${unameOut}"
esac

if [ $# -eq 0 ]
  then
    echo "no arguments supplied, exiting"
    exit 1
fi

if [ -z "$1" ]; then
    echo "no template supplied, exiting"
    exit 1
fi

if [ -z "${BIN}" ]; then
    echo "BIN not set, exiting"
    exit 1
fi

BIN=`echo "${BIN}" | tr '[:lower:]' '[:upper:]'`

while IFS='=' read -r name value ; do
  if [[ $name == *"${BIN}_"* ]]; then
    if [ "$machine" = "Mac" ]; then
        sed -i '' -e "s#{{ ${name} }}#${!name}#g" $1
    else
        sed -i "s#{{ ${name} }}#${!name}#g" $1
    fi
  fi
done < <(env)
