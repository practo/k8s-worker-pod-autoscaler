# Upgrade Worker Pod Autoscaler

## Upgrade from v1.2 to v1.3

### Breaking changes
Updates all the kubernetes dependencies with `v1.19`. It should work for the cluster with older versions, but Kubernetes supports patches and fixes for [last 3 minor releases](https://kubernetes.io/docs/setup/release/version-skew-policy/). It also updates the CRD definitions.

### Recommended Actions
```
kubectl apply -f ./artifacts/crd.yaml
```
### Changes
- [v1.3.0](https://github.com/practo/k8s-worker-pod-autoscaler/releases/tag/v1.3.0)


## Upgrade from v1.1 to v1.2

### Breaking changes
`v1alpha1` was discontinued. Please move to `v1`.

### Recommended Actions
```
kubectl apply -f ./artifacts/crd.yaml
```
### Changes
- [v1.2.0](https://github.com/practo/k8s-worker-pod-autoscaler/releases/tag/v1.2.0)

## Upgrade from v1.0 to v1.1

### Breaking Changes
There is no backward breaking change from `v1.0` to `v1.1`.

### Recommended Actions
Update the WorkerPodAutoScaler Clusterrole to give WPA access to fetch Replicaset and also the CRD.
```
kubectl apply -f ./artifacts/clusterrole.yaml
kubectl apply -f ./artifacts/crd.yaml
```

Note: Support for `v1alpha1` will be discontinued from `v1.2`.

### Changes
- [v1.1.0](https://github.com/practo/k8s-worker-pod-autoscaler/releases/tag/v1.1.0)

## Upgrade from v0.2 to v1.0

### Breaking Changes
There is no backward breaking change from `v0.2` to `v1`.

### Recommended Actions
Update the WorkerPodAutoScaler CRD from `v1alpha1` to `v1` using below:
```
kubectl apply -f ./artifacts/crd.yaml
```

Note: Support for `v1alpha1` CRD version is still there, but it would be discontinued in the future releases.

### Changes
- [v1.0.0](https://github.com/practo/k8s-worker-pod-autoscaler/releases/tag/v1.0.0)
- [v1.0.0-beta](https://github.com/practo/k8s-worker-pod-autoscaler/releases/tag/v1.0.0-beta)
