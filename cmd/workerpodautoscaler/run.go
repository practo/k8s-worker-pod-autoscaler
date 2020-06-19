package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/apis/workerpodautoscaler/v1alpha1"
	"github.com/practo/k8s-worker-pod-autoscaler/pkg/cmdutil"
	"github.com/practo/k8s-worker-pod-autoscaler/pkg/signals"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	flags := v.Cmd.Flags()

	flagNames := []string{
		"resync-period",
		"wpa-threads",
		"wpa-default-max-disruption",
		"aws-regions",
		"kube-config",
		"sqs-short-poll-interval",
		"sqs-long-poll-interval",
		"beanstalk-short-poll-interval",
		"beanstalk-long-poll-interval",
		"queue-services",
		"metrics-port",
		"k8s-api-qps",
		"k8s-api-burst",
	}

	flags.Int("resync-period", 20, "sync period for the worker pod autoscaler")
	flags.Int("wpa-threads", 10, "wpa threadiness, number of threads to process wpa resources")
	flags.String("wpa-default-max-disruption", "100%", "it is the default value for the maxDisruption in the WPA spec. This specifies how much percentage of pods can be disrupted in a single scale down acitivity. Can be expressed as integers or as a percentage.")
	flags.String("aws-regions", "ap-south-1,ap-southeast-1", "comma separated aws regions of SQS")
	flags.String("kube-config", "", "path of the kube config file, if not specified in cluster config is used")
	flags.Int("sqs-short-poll-interval", 20, "the duration (in seconds) after which the next sqs api call is made to fetch the queue length")
	flags.Int("sqs-long-poll-interval", 20, "the duration (in seconds) for which the sqs receive message call waits for a message to arrive")
	flags.Int("beanstalk-short-poll-interval", 20, "the duration (in seconds) after which the next beanstalk api call is made to fetch the queue length")
	flags.Int("beanstalk-long-poll-interval", 20, "the duration (in seconds) for which the beanstalk receive message call waits for a message to arrive")
	flags.String("queue-services", "sqs,beanstalkd", "comma separated queue services, the WPA will start with")

	flags.String("metrics-port", ":8787", "specify where to serve the /metrics and /status endpoint. /metrics serve the prometheus metrics for WPA")
	flags.Float64("k8s-api-qps", 5.0, "qps indicates the maximum QPS to the k8s api from the clients(wpa).")
	flags.Int("k8s-api-burst", 10, "maximum burst for throttle between requests from clients(wpa) to k8s api")
	for _, flagName := range flagNames {
		if err := v.BindFlag(flagName); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	return v.Cmd
}

func parseRegions(regionNames string) []string {
	var awsRegions []string
	regions := strings.Split(regionNames, ",")
	for _, region := range regions {
		awsRegions = append(awsRegions, strings.TrimSpace(region))
	}
	return awsRegions
}

func (v *runCmd) run(cmd *cobra.Command, args []string) {
	resyncPeriod := time.Second * time.Duration(
		v.Viper.GetInt("resync-period"))
	wpaThraeds := v.Viper.GetInt("wpa-threads")
	wpaDefaultMaxDisruption := v.Viper.GetString("wpa-default-max-disruption")
	awsRegions := parseRegions(v.Viper.GetString("aws-regions"))
	kubeConfigPath := v.Viper.GetString("kube-config")
	sqsShortPollInterval := v.Viper.GetInt("sqs-short-poll-interval")
	sqsLongPollInterval := v.Viper.GetInt("sqs-long-poll-interval")
	beanstalkShortPollInterval := v.Viper.GetInt(
		"beanstalk-short-poll-interval")
	beanstalkLongPollInterval := v.Viper.GetInt("beanstalk-long-poll-interval")
	queueServicesToStartWith := v.Viper.GetString("queue-services")
	metricsPort := v.Viper.GetString("metrics-port")
	k8sApiQPS := float32(v.Viper.GetFloat64("k8s-api-qps"))
	k8sApiBurst := v.Viper.GetInt("k8s-api-burst")

	// // set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	// cfg, err := createRestConfig("")
	cfg, err := createRestConfig(kubeConfigPath)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	cfg.QPS = k8sApiQPS
	cfg.Burst = k8sApiBurst

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
	go queues.Sync(stopCh)

	var queuingServices []queue.QueuingService

	// Make all the message service providers and start their pollers
	for _, q := range strings.Split(queueServicesToStartWith, ",") {
		q = strings.TrimSpace(q)
		switch q {
		case queue.SqsQueueService:
			sqs, err := queue.NewSQS(
				queue.SqsQueueService,
				awsRegions, queues, sqsShortPollInterval, sqsLongPollInterval)
			if err != nil {
				klog.Fatalf("Error creating sqs Poller: %v", err)
			}

			queuingServices = append(queuingServices, sqs)
		case queue.BeanstalkQueueService:
			bs, err := queue.NewBeanstalk(
				queue.BeanstalkQueueService,
				queues, beanstalkShortPollInterval, beanstalkLongPollInterval)
			if err != nil {
				klog.Fatalf("Error creating bs Poller: %v", err)
			}

			queuingServices = append(queuingServices, bs)
		default:
			klog.Fatal("Unsupported queue provider: ", q)
		}
	}

	for _, queuingService := range queuingServices {
		go queuingService.Sync(stopCh)
		poller := queue.NewPoller(queues, queuingService)
		go poller.Sync(stopCh)
		go poller.Run(stopCh)
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(
		kubeClient, resyncPeriod)
	customInformerFactory := informers.NewSharedInformerFactory(
		customClient, resyncPeriod)

	controller := workerpodautoscalercontroller.NewController(
		kubeClient, customClient,
		kubeInformerFactory.Apps().V1().Deployments(),
		customInformerFactory.K8s().V1alpha1().WorkerPodAutoScalers(),
		wpaDefaultMaxDisruption,
		queues,
	)

	// notice that there is no need to run Start methods in a
	// separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// Start method is non-blocking and runs all registered
	// informers in a dedicated goroutine.
	kubeInformerFactory.Start(stopCh)
	customInformerFactory.Start(stopCh)

	go serveMetrics(metricsPort)

	// TODO: autoscale the worker threads based on number of
	// queues registred in WPA
	if err = controller.Run(wpaThraeds, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
	return
}

func serveMetrics(metricsPort string) {
	http.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(metricsPort, nil)
}

func createRestConfig(kubeConfigPath string) (*rest.Config, error) {
	if kubeConfigPath == "" {
		config, err := rest.InClusterConfig()
		return config, err
	}
	return clientcmd.BuildConfigFromFlags("", kubeConfigPath)
}
