package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/practo/klog/v2"
	"github.com/practo/promlog"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/cmdutil"
	"github.com/practo/k8s-worker-pod-autoscaler/pkg/signals"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	workerpodautoscalercontroller "github.com/practo/k8s-worker-pod-autoscaler/pkg/controller"
	clientset "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/clientset/versioned"
	informers "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/informers/externalversions"
	queue "github.com/practo/k8s-worker-pod-autoscaler/pkg/queue"
	statsig "github.com/statsig-io/go-sdk"
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
		"scale-down-delay-after-last-scale-activity",
		"resync-period",
		"wpa-threads",
		"wpa-default-max-disruption",
		"aws-regions",
		"kube-config",
		"sqs-short-poll-interval",
		"sqs-long-poll-interval",
		"queue-services",
		"metrics-port",
		"k8s-api-qps",
		"k8s-api-burst",
		"namespace",
	}

	flags.Int("scale-down-delay-after-last-scale-activity", 600, "scale down delay after last scale up or down in seconds")
	flags.Int("resync-period", 20, "maximum sync period for the control loop but the control loop can execute sooner if the wpa status object gets updated.")
	flags.Int("wpa-threads", 10, "wpa threadiness, number of threads to process wpa resources")
	flags.String("wpa-default-max-disruption", "100%", "it is the default value for the maxDisruption in the WPA spec. This specifies how much percentage of pods can be disrupted in a single scale down acitivity. Can be expressed as integers or as a percentage.")
	flags.String("aws-regions", "us-east-1", "comma separated aws regions of SQS")
	flags.String("kube-config", "", "path of the kube config file, if not specified in cluster config is used")
	flags.Int("sqs-short-poll-interval", 20, "the duration (in seconds) after which the next sqs api call is made to fetch the queue length")
	flags.Int("sqs-long-poll-interval", 20, "the duration (in seconds) for which the sqs receive message call waits for a message to arrive")
	flags.String("queue-services", "sqs", "comma separated queue services, the WPA will start with")
	flags.String("metrics-port", ":8787", "specify where to serve the /metrics and /status endpoint. /metrics serve the prometheus metrics for WPA")
	flags.Float64("k8s-api-qps", 5.0, "qps indicates the maximum QPS to the k8s api from the clients(wpa).")
	flags.Int("k8s-api-burst", 10, "maximum burst for throttle between requests from clients(wpa) to k8s api")

	flags.String("namespace", "", "specify the namespace to listen to")
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
	statsig.InitializeWithOptions(v.Viper.GetString("STATSIG_SDK_KEY"), &statsig.Options{Environment: statsig.Environment{Tier: "staging"}})
	scaleDownDelay := time.Second * time.Duration(
		v.Viper.GetInt("scale-down-delay-after-last-scale-activity"),
	)
	resyncPeriod := time.Second * time.Duration(
		v.Viper.GetInt("resync-period"),
	)
	wpaThraeds := v.Viper.GetInt("wpa-threads")
	wpaDefaultMaxDisruption := v.Viper.GetString("wpa-default-max-disruption")
	awsRegions := parseRegions(v.Viper.GetString("aws-regions"))
	kubeConfigPath := v.Viper.GetString("kube-config")
	sqsShortPollInterval := v.Viper.GetInt("sqs-short-poll-interval")
	sqsLongPollInterval := v.Viper.GetInt("sqs-long-poll-interval")
	queueServicesToStartWith := v.Viper.GetString("queue-services")
	metricsPort := v.Viper.GetString("metrics-port")
	k8sApiQPS := float32(v.Viper.GetFloat64("k8s-api-qps"))
	k8sApiBurst := v.Viper.GetInt("k8s-api-burst")
	namespace := v.Viper.GetString("namespace")

	hook := promlog.MustNewPrometheusHook("wpa_", klog.WarningSeverityLevel)
	klog.AddHook(hook)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		default:
			klog.Fatal("Unsupported queue provider: ", q)
		}
	}

	for _, queuingService := range queuingServices {
		poller := queue.NewPoller(queues, queuingService)
		go poller.Sync(stopCh)
		go poller.Run(stopCh)
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		kubeClient, resyncPeriod, kubeinformers.WithNamespace(namespace))
	customInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		customClient, resyncPeriod, informers.WithNamespace(namespace))

	controller := workerpodautoscalercontroller.NewController(
		ctx, kubeClient, customClient,
		kubeInformerFactory.Apps().V1().Deployments(),
		kubeInformerFactory.Apps().V1().ReplicaSets(),
		customInformerFactory.K8s().V1().WorkerPodAutoScalerMultiQueues(),
		wpaDefaultMaxDisruption,
		resyncPeriod,
		scaleDownDelay,
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
