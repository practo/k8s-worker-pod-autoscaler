package main

import (
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/apis/workerpodautoscaler/v1alpha1"
	"github.com/practo/k8s-worker-pod-autoscaler/pkg/cmdutil"
	"github.com/practo/k8s-worker-pod-autoscaler/pkg/signals"
	"github.com/spf13/cobra"

	workerpodautoscalercontroller "github.com/practo/k8s-worker-pod-autoscaler/pkg/controller"
	clientset "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/clientset/versioned"
	informers "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/informers/externalversions"
	queue "github.com/practo/k8s-worker-pod-autoscaler/pkg/queue"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

type runCmd struct {
	cmdutil.BaseCmd
}

var (
	runLong    = `Run the workerpodautoscaler`
	runExample = `  workerpodautoscaler run`
)

func (v *runCmd) new() *cobra.Command {
	v.Init("workerpodautoscaler", &cobra.Command{
		Use:     "run",
		Short:   "Run the workerpodautoscaler",
		Long:    runLong,
		Example: runExample,
		Run:     v.run,
	})

	return v.Cmd
}

func (v *runCmd) run(cmd *cobra.Command, args []string) {
	// // set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	// cfg, err := createRestConfig("")
	cfg, err := createRestConfig("/Users/alok87/.kube/config")
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	customClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building custom clientset: %s", err.Error())
	}

	apiExtensionClient, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error creating api extension client: %s", err.Error())
	}

	err = v1alpha1.CreateCRD(apiExtensionClient)
	if err != nil {
		klog.Fatalf("Error creating crd: %s", err.Error())
	}

	queues := queue.NewQueues()
	go queues.Sync()
	go queues.SyncLister()

	sqsPoller, err := queue.NewSQSPoller("ap-south-1", queues)
	if err != nil {
		klog.Fatalf("Error creating sqs Poller: %v", err)
	}
	go sqsPoller.Run()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	customInformerFactory := informers.NewSharedInformerFactory(customClient, time.Second*30)

	controller := workerpodautoscalercontroller.NewController(kubeClient, customClient,
		kubeInformerFactory.Apps().V1().Deployments(),
		customInformerFactory.K8s().V1alpha1().WorkerPodAutoScalers(),
		queues,
	)

	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	kubeInformerFactory.Start(stopCh)
	customInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
	return
}

func createRestConfig(kubeConfigPath string) (*rest.Config, error) {
	if kubeConfigPath == "" {
		config, err := rest.InClusterConfig()
		return config, err
	}
	return clientcmd.BuildConfigFromFlags("", kubeConfigPath)
}
