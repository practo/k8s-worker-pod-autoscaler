# Upgrade Worker Pod Autoscaler

## Upgrade from v0.2 to v1

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
