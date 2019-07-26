# Kubernetes Worker Pod Autoscaler [WPA]

Scale kubernetes deployments based on AWS SQS queue length and beanstalk queues. Scaling based on other queues is also possible, but integration work with this controller is required for them(future releases).

```
kubectl get wpa
```

## What do you mean by worker?
Worker are jobs or long running processes or pods which watches over a queue to do the job. Worker could be a Kubernetes Deployment or a Kubernetes Job(not supported, at present).

## Why build a custom controller ?

Kubernetes is a wonderful platform. But it does not solve all the use case, this is the reason CRDs exist.

Horizontal Pod autoscaler and custom metric scaling kubernetes does not work well when you need to scale based on SQS queue length. Following are the reasons for building a custom controller.

1) The scaling up and down metric for queue based scaling are different and depends on the queue service provider. For example in SQS, scale up of kubernetes deployments can be done based on "ApproximateMessagesVisible" but scaling down to zero cannot be done on that metric. So if the queue is getting consumed very fast the ApproximateMessagesVisible will always be zero, so "NumberOfEmptyReceives" is the right metric to scale things down.

2) Using HPA and custom metrics requires exporting custom metric from various sources and making it available for scaling. So if your use case is queue based scaling this controller can help you

If you run this controller in your cluster, there is no need to write custom cloudwatch or AWS SQS exporters and make it work with the kubernetes HPA. Run this controller with correct AWS access permissions and you can start scaling your worker deployments based on queue length.

## Good things

1) **OnDemand Kubernetes Deployments/ Server-less workers, min=0:** Make your kubernetes deployments on-demand using `min=0` with *minimum effort*.

2) **Speed:** For AWS Users, since this works on SQS metric, there is no lag in scaling. If you use cloudwatch metric for SQS then there is a 10minute lag since it takes 10minutes for SQS metrics to reflect on Cloudwatch.

## How to use it?

1) Run the controller (this creates the CRD if not present and starts the controller)
```
kubectl create -f hack/workerpodautoscaler-controller.yaml
```

2) Create the Worker Pod Autoscaler custom resource defination speciying your scale up and down configuration.
```
kubectl create -f hack/wpa.yaml
```
