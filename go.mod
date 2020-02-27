module github.com/practo/k8s-worker-pod-autoscaler

go 1.12

require (
	9fans.net/go v0.0.2 // indirect
	github.com/aws/aws-sdk-go v1.21.8
	github.com/beanstalkd/go-beanstalk v0.0.0-20190515041346-390b03b3064a // indirect
	github.com/kisielk/errcheck v1.2.0 // indirect
	github.com/kr/beanstalk v0.0.0-20180818045031-cae1762e4858 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mpdroog/beanstalkd v0.0.0-20151231090228-bf387addf002
	github.com/rogpeppe/godef v1.1.1 // indirect
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	github.com/spf13/viper v1.4.0
	golang.org/x/tools v0.0.0-20200226224502-204d844ad48d // indirect
	k8s.io/api v0.0.0-20190726022912-69e1bce1dad5
	k8s.io/apiextensions-apiserver v0.0.0-20190726024412-102230e288fd
	k8s.io/apimachinery v0.0.0-20190726022757-641a75999153
	k8s.io/client-go v0.0.0-20190726023111-a9c895e7f2ac
	k8s.io/code-generator v0.0.0-20190726022633-14ba7d03f06f
	k8s.io/klog v0.3.1
)

replace (
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.0
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20181025213731-e84da0312774
	golang.org/x/net => golang.org/x/net v0.0.0-20190206173232-65e2d4e15006
	golang.org/x/sync => golang.org/x/sync v0.0.0-20181108010431-42b317875d0f
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190209173611-3b5209105503
	golang.org/x/text => golang.org/x/text v0.3.1-0.20181227161524-e6919f6577db
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190313210603-aa82965741a9
	k8s.io/api => k8s.io/api v0.0.0-20190819141258-3544db3b9e44
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190819143637-0dbe462fe92d
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190819141724-e14f31a72a77
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b
)
