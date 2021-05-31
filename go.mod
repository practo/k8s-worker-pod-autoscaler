module github.com/practo/k8s-worker-pod-autoscaler

go 1.16

require (
	github.com/aws/aws-sdk-go v1.38.51
	github.com/beanstalkd/go-beanstalk v0.1.0
	github.com/golang/mock v1.5.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/practo/klog/v2 v2.2.1
	github.com/practo/promlog v1.0.0
	github.com/prometheus/client_golang v1.10.0
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	k8s.io/api v0.20.6
	k8s.io/apimachinery v0.20.6
	k8s.io/client-go v0.20.6
	k8s.io/code-generator v0.20.6
)
