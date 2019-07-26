package main

import (
	// "time"

	// "k8s.io/klog"
	"k8s.io/client-go/rest"
	// "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	// kubeinformers "k8s.io/client-go/informers"

	"github.com/spf13/cobra"
	// "github.com/practo/k8s-worker-pod-autoscaler"
	"github.com/practo/k8s-worker-pod-autoscaler/pkg/cmdutil"

	// workerpodautoscalercontroller "github.com/practo/k8s-worker-pod-autoscaler/pkg/controller"
	// clientset "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/clientset/versioned"
    // samplescheme "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/clientset/versioned/scheme"
    // listers "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/listers/workerpodautoscaler/v1alpha1"
    // informers "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/informers/externalversions/workerpodautoscaler/v1alpha1"
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
	// klog.InitFlags(nil)
	//
	// // set up signals so we handle the first shutdown signal gracefully
	// stopCh := signals.SetupSignalHandler()
	//
	// // cfg, err := createRestConfig("")
	// cfg, err := createRestConfig("/Users/alok87/.kube/config")
	// if err != nil {
	// 	glog.Fatalf("Error building kubeconfig: %s", err.Error())
	// }
	//
	// kubeClient, err := kubernetes.NewForConfig(cfg)
	// if err != nil {
	// 	glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	// }
	//
	// customClient, err := clientset.NewForConfig(cfg)
	// if err != nil {
	// 	glog.Fatalf("Error building custom clientset: %s", err.Error())
	// }
	//
	// kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	// customInformerFactory := informers.NewSharedInformerFactory(customClient, time.Second*30)
	//
	// controller := workerpodautoscalercontroller.NewController(kubeClient, exampleClient,
	// 	// kubeInformerFactory.Apps().V1().Deployments(),
	// 	customInformerFactory.Samplecontroller().V1alpha1().Foos()
	// )
	//
	// // notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// // Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	// kubeInformerFactory.Start(stopCh)
	// exampleInformerFactory.Start(stopCh)
	//
	// if err = controller.Run(2, stopCh); err != nil {
	// 	klog.Fatalf("Error running controller: %s", err.Error())
	// }
	return
}

func createClientConfig(kubeConfigPath string) (*rest.Config, error) {
	if kubeConfigPath == "" {
		config, err := rest.InClusterConfig()
		return config, err
	}
	return clientcmd.BuildConfigFromFlags("", kubeConfigPath)
}
