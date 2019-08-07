#!/bin/bash

set -e

template_deployment='./artifacts/deployment-template.yaml'
new_deployment='./artifacts/deployment.yaml'

cp -f $template_deployment $new_deployment

echo "Substituting variables in $template_deployment to create $new_deployment"
if [ -z "${AWS_REGION}" ]; then
    echo "AWS_REGION not set in environment, exiting"
    exit 1
fi
if [ -z "${AWS_ACCESS_KEY_ID}" ]; then
    echo "AWS_ACCESS_KEY_ID not set in environment, exiting"
    exit 1
fi
if [ -z "${AWS_SECRET_ACCESS_KEY}" ]; then
    echo "AWS_SECRET_ACCESS_KEY not set in environment, exiting"
    exit 1
fi

perl -i -wpe "s/{{ WPA_AWS_REGION }}/${AWS_REGION}/g" ${new_deployment}
perl -i -wpe "s/{{ WPA_AWS_ACCESS_KEY_ID }}/${AWS_ACCESS_KEY_ID}/g" ${new_deployment}
perl -i -wpe "s/{{ WPA_AWS_SECRET_ACCESS_KEY }}/${AWS_SECRET_ACCESS_KEY}/g" ${new_deployment}

cat ${new_deployment}

echo "Applying worker pod autoscaler deployment in kubernetes."
kubectl apply -f ${new_deployment}
