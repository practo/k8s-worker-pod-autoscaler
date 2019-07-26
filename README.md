# Kubernetes SQS POD Autoscaler

SQS pod autoscaler to scale up and down pods based on SQS queue length

```
kubectl get spa
```

## Why build a custom controller ?

Kubernetes Horizontal Pod autoscaler and custom metric scaling kubernetes does not work well when you need to scale based on SQS queue length. Following are the reasons for building a custom controller.

1) The scaling up and down metric for SQS based scaling are different. Scale up of kubernetes deployments can be done based on "ApproximateMessagesVisible" but scaling down to zero cannot be done on that metric. So if the queue is getting consumed very fast the ApproximateMessagesVisible will always be zero, so "NumberOfEmptyReceives" is the right metric to scale things down.

2) Running this controller is enough there is no need to write custom cloudwatch or AWS SQS exporters and make it work with the kubernetes HPA. Run this controller with correct AWS access permission and you can start scaling your deployments based on SQS queue length.

## Use case

1) On-demand Kubernetes deployments/ Server-less workers, min=0: Make your kubernetes deployments on-demand with *minimum effort*.

2) Speed: Since this works on SQS metric, there is no lag in scaling. If you use cloudwatch metric for SQS then there is a 10minute lag since it takes 10minutes for SQS metrics to reflect on Cloudwatch.

## How to use it?

1) Run the controller (this creates the CRD if not present and starts the controller)
```
kubectl create -f hack/sqspodautoscaler-controller.yaml
```

2) Create the SQS Pod Autoscaler custom resource defination speciying your scale up and down configuration.
```
kubectl create -f hack/spa.yaml
```
