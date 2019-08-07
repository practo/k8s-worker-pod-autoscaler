#!/bin/bash

set -e

serviceaccount='./artifacts/serviceaccount.yaml'
clusterrole='./artifacts/clusterrole.yaml'
clusterrolebinding='./artifacts/clusterrolebinding.yaml'
new_deployment='./artifacts/deployment.yaml'
template_deployment='./artifacts/deployment-template.yaml'

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

cp -f $template_deployment $new_deployment
perl -i -pe "s#{WPA_AWS_REGION}#${AWS_REGION}#g" ${new_deployment}
perl -i -pe "s#{WPA_AWS_ACCESS_KEY_ID}#/${AWS_ACCESS_KEY_ID}#g" ${new_deployment}
perl -i -pe "s#{WPA_AWS_SECRET_ACCESS_KEY}#/${AWS_SECRET_ACCESS_KEY}#g" ${new_deployment}

kubectl apply -f ${serviceaccount}
kubectl apply -f ${clusterrole}
kubectl apply -f ${clusterrolebinding}
kubectl apply -f ${new_deployment}
kubectl get pods -n kube-system | grep workerpodautoscaler
