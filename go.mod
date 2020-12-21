module github.com/practo/k8s-worker-pod-autoscaler

go 1.15

require (
	github.com/aws/aws-sdk-go v1.36.12
	github.com/beanstalkd/go-beanstalk v0.1.0
	github.com/golang/mock v1.4.4
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/practo/klog/v2 v2.2.1
	github.com/practo/promlog v1.0.0
	github.com/prometheus/client_golang v1.9.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	k8s.io/api v0.19.0
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v0.19.0
	k8s.io/code-generator v0.19.0
)
