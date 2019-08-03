# Worker Pod Autoscaler
Worker Pod Autoscaler automatically scales the number of pods in a deployment based on observed queue length in AWS SQS or Beanstalk tubes.

Currently AWS SQS and Beanstalk are supported.

# Install the WorkerPodAutoscaler

### Install
Running the below script will create the WPA CRD and start the controller. The controller watches over all the specified queues in AWS and beanstalk and scales the Kubernetes deployments based on the specification.

```bash
export AWS_ACCESS_KEY_ID='sample-aws-access-key-id'
export AWS_SECRET_ACCESS_KEY='sample-aws-secret-acesss-key'
./hack/install.sh
```

### Verify Installation
Check the wpa resource is accessible using kubectl

```bash
kubectl get wpa
```

# Example
Please install following above before trying the below example.

- Create Deployment that needs to scale based on queue length.
```bash
kubectl create -f artificats/example-deployment.yaml
```

- Create `WPA object (example-wpa)` that will scale the `example-deployment` based on SQS queue length.
```bash
kubectl create -f artifacts/example-wpa.yaml
```

This will start scaling `example-deployment` based on SQS queue length.

# Why?

Kubernetes does support custom metric scaling using Horizontal Pod Autoscaler. Before making this we were using HPA to scale our worker pods. Below are the reasons for moving away from HPA and making a custom resource:

**TLDR;** Don't want to write and maintain custom metric exporters? Use WPA to quickly start scaling your pods based on queue length with minimum effort (few kubectl commands and you are done !)

1. **No need to write and maintain custom metric exporters**: In case of HPA with custom metrics, the users need to write and maintain the custom metric exporters. This makes sense for HPA to support all kinds of use cases. WPA comes with queue metric exporters integrated and can be put into use with few kubectl commands.

2. **Different Metrics for Scaling Up and Down**: Scaling up and down metric can be different based on the use case. For example in our case we want to scale up based on SQS `ApproximateMessageVisible` length and scale down based on `NumberOfEmptyReceive`. This is because if the worker jobs watching the queue is consuming the queue very fast, `ApproximateMessageVisible` would always be zero and you don't want to scale down to 0 in such cases.

3. **Fast**: The scaling should happen as soon as the queue metric changes. It makes use of Golang concurrency model using go-routines and channels to make this as close to real time as possible.

4. **Ondemand workers:** min=0 is support (its also supported in HPA now)
