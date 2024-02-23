package controller

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/practo/klog/v2"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"

	v1 "github.com/practo/k8s-worker-pod-autoscaler/pkg/apis/workerpodautoscalermultiqueue/v1"
	clientset "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/clientset/versioned"
	samplescheme "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/clientset/versioned/scheme"
	informers "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/informers/externalversions/workerpodautoscalermultiqueue/v1"
	listers "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/listers/workerpodautoscalermultiqueue/v1"
	queue "github.com/practo/k8s-worker-pod-autoscaler/pkg/queue"
)

const controllerAgentName = "workerpodcustomautoscaler-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a WorkerPodAutoScalerMultiQueue is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a WorkerPodAutoScalerMultiQueue fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by WorkerPodAutoScalerMultiQueue"
	// MessageResourceSynced is the message used for an Event fired when a WorkerPodAutoScalerMultiQueue
	// is synced successfully
	MessageResourceSynced = "WorkerPodAutoScalerMultiQueue synced successfully"

	// WokerPodAutoScalerEventAdd stores the add event name
	WokerPodAutoScalerEventAdd = "add"

	// WokerPodAutoScalerEventUpdate stores the add event name
	WokerPodAutoScalerEventUpdate = "update"

	// WokerPodAutoScalerEventDelete stores the add event name
	WokerPodAutoScalerEventDelete = "delete"

	// PausedQueuesDynamicConfigName is the name of statsig dynamic config for paused queues
	PausedQueuesDynamicConfigName = "platform-shoryuken-paused-queues"
)

var (
	loopDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wpa",
			Subsystem: "controller",
			Name:      "loop_duration_seconds",
			Help:      "Number of seconds to complete the control loop successfully, partitioned by wpa name and namespace",
		},
		[]string{"workerpodcustomautoscaler", "namespace"},
	)

	loopCountSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "wpa",
			Subsystem: "controller",
			Name:      "loop_count_success",
			Help:      "How many times the control loop executed successfully, partitioned by wpa name and namespace",
		},
		[]string{"workerpodcustomautoscaler", "namespace"},
	)

	qMsgs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wpa",
			Subsystem: "queue",
			Name:      "messages",
			Help:      "Number of unprocessed messages in the queue",
		},
		[]string{"workerpodcustomautoscaler", "namespace", "queueName"},
	)

	qMsgsSPM = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wpa",
			Subsystem: "queue",
			Name:      "messages_sent_per_minute",
			Help:      "Number of messages sent to the queue per minute",
		},
		[]string{"workerpodcustomautoscaler", "namespace", "queueName"},
	)

	workersIdle = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wpa",
			Subsystem: "worker",
			Name:      "idle",
			Help:      "Number of idle workers",
		},
		[]string{"workerpodcustomautoscaler", "namespace", "deploymentName"},
	)

	workersCurrent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wpa",
			Subsystem: "worker",
			Name:      "current",
			Help:      "Number of current workers",
		},
		[]string{"workerpodcustomautoscaler", "namespace", "deploymentName"},
	)

	workersDesired = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wpa",
			Subsystem: "worker",
			Name:      "desired",
			Help:      "Number of desired workers",
		},
		[]string{"workerpodcustomautoscaler", "namespace", "deploymentName"},
	)

	workersAvailable = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wpa",
			Subsystem: "worker",
			Name:      "available",
			Help:      "Number of available workers",
		},
		[]string{"workerpodcustomautoscaler", "namespace", "deploymentName"},
	)
)

func init() {
	prometheus.MustRegister(loopDurationSeconds)
	prometheus.MustRegister(loopCountSuccess)
	prometheus.MustRegister(qMsgs)
	prometheus.MustRegister(qMsgsSPM)
	prometheus.MustRegister(workersIdle)
	prometheus.MustRegister(workersCurrent)
	prometheus.MustRegister(workersDesired)
	prometheus.MustRegister(workersAvailable)
}

type WokerPodAutoScalerEvent struct {
	key  string
	name string
}

// Controller is the controller implementation for WorkerPodAutoScalerMultiQueue resources
type Controller struct {
	ctx context.Context

	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// customclientset is a clientset for our own API group
	customclientset            clientset.Interface
	deploymentLister           appslisters.DeploymentLister
	deploymentsSynced          cache.InformerSynced
	replicaSetLister           appslisters.ReplicaSetLister
	replicaSetsSynced          cache.InformerSynced
	workerPodAutoScalersLister listers.WorkerPodAutoScalerMultiQueueLister
	workerPodAutoScalersSynced cache.InformerSynced
	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
	// defaultMaxDisruption
	// it is the default value for the maxDisruption in the WPA spec.
	// This specifies how much percentage of pods can be disrupted in a
	// single scale down acitivity.
	// Can be expressed as integers or as a percentage.
	defaultMaxDisruption string
	// QueueList keeps the list of all the queues in memeory
	// which is used by the core controller and the sqs exporter

	// scaleDownDelay after last scale up
	// the no of seconds to wait after the last scale up before scaling down
	scaleDownDelay time.Duration

	Queues *queue.Queues
}

// NewController returns a new sample controller
func NewController(
	ctx context.Context,
	kubeclientset kubernetes.Interface,
	customclientset clientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	replicaSetInformer appsinformers.ReplicaSetInformer,
	workerPodAutoScalerInformer informers.WorkerPodAutoScalerMultiQueueInformer,
	defaultMaxDisruption string,
	resyncPeriod time.Duration,
	scaleDownDelay time.Duration,
	queues *queue.Queues) *Controller {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		ctx:                        ctx,
		kubeclientset:              kubeclientset,
		customclientset:            customclientset,
		deploymentLister:           deploymentInformer.Lister(),
		deploymentsSynced:          deploymentInformer.Informer().HasSynced,
		replicaSetLister:           replicaSetInformer.Lister(),
		replicaSetsSynced:          replicaSetInformer.Informer().HasSynced,
		workerPodAutoScalersLister: workerPodAutoScalerInformer.Lister(),
		workerPodAutoScalersSynced: workerPodAutoScalerInformer.Informer().HasSynced,
		workqueue:                  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "WorkerPodAutoScalers"),
		recorder:                   recorder,
		defaultMaxDisruption:       defaultMaxDisruption,
		scaleDownDelay:             scaleDownDelay,
		Queues:                     queues,
	}

	klog.V(4).Info("Setting up event handlers")

	// Set up an event handler for when WorkerPodAutoScalerMultiQueue resources change
	workerPodAutoScalerInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueAddWorkerPodAutoScaler,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueUpdateWorkerPodAutoScaler(new)
		},
		DeleteFunc: controller.enqueueDeleteWorkerPodAutoScaler,
	}, resyncPeriod)
	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.V(1).Info("Starting WorkerPodAutoScalerMultiQueue controller")

	// Wait for the caches to be synced before starting workers
	klog.V(1).Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.deploymentsSynced, c.workerPodAutoScalersSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.V(1).Info("Starting workers")
	// Launch two workers to process WorkerPodAutoScalerMultiQueue resources
	for i := 0; i < threadiness; i++ {
		// TOOD: move from stopCh to context, use: UntilWithContext()
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	<-stopCh
	klog.V(1).Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem(c.ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.(PS: not anymore, its an WPA event)

		event, ok := obj.(WokerPodAutoScalerEvent)
		if !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// WorkerPodAutoScalerMultiQueue resource to be synced.
		if err := c.syncHandler(ctx, event); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(event)
			return fmt.Errorf("error syncing '%s': %s, requeuing", event, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the WorkerPodAutoScalerMultiQueue resource
// with the current status of the resource.
func (c *Controller) syncHandler(ctx context.Context, event WokerPodAutoScalerEvent) error {
	now := time.Now()
	key := event.key
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the WorkerPodAutoScalerMultiQueue resource with this namespace/name
	workerPodAutoScaler, err := c.workerPodAutoScalersLister.WorkerPodAutoScalerMultiQueues(namespace).Get(name)
	if err != nil {
		// The WorkerPodAutoScalerMultiQueue resource may no longer exist, in which case we stop processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("workerPodAutoScaler '%s' in work queue no longer exists", key))
			c.Queues.Delete(namespace, name, "")
			return nil
		}
		return err
	}

	var currentWorkers, availableWorkers int32
	deploymentName := workerPodAutoScaler.Spec.DeploymentName
	if deploymentName != "" {
		// Get the Deployment with the name specified in WorkerPodAutoScalerMultiQueue.spec
		deployment, err := c.deploymentLister.Deployments(workerPodAutoScaler.Namespace).Get(deploymentName)
		if errors.IsNotFound(err) {
			return fmt.Errorf("deployment %s not found in namespace %s",
				deploymentName, workerPodAutoScaler.Namespace)
		} else if err != nil {
			return err
		}
		currentWorkers = *deployment.Spec.Replicas
		availableWorkers = deployment.Status.AvailableReplicas
	} else {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		utilruntime.HandleError(fmt.Errorf("%s: deployment or replicaset name must be specified", key))
		return nil
	}

	switch event.name {
	case WokerPodAutoScalerEventAdd:
		for _, q := range workerPodAutoScaler.Spec.Queues {
			err = c.Queues.Add(
				namespace,
				name,
				q.URI,
				currentWorkers,
				q.SecondsToProcessOneJob,
			)
		}
	case WokerPodAutoScalerEventUpdate:
		for _, q := range workerPodAutoScaler.Spec.Queues {
			err = c.Queues.Add(
				namespace,
				name,
				q.URI,
				currentWorkers,
				q.SecondsToProcessOneJob,
			)
		}
	case WokerPodAutoScalerEventDelete:
		for _, q := range workerPodAutoScaler.Spec.Queues {
			qName := queue.GetQueueName(q.URI)
			err = c.Queues.Delete(namespace, name, qName)
		}
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to sync queue: %s", err.Error()))
		return err
	}

	qSpecs := c.Queues.ListMultiQueues(key)
	for _, qSpec := range qSpecs {
		if qSpec.Messages == queue.UnsyncedQueueMessageCount {
			klog.Warningf(
				"%s qMsgs: %d, q not initialized, waiting for init to complete",
				qSpec.Name,
				qSpec.Messages,
			)
			return nil
		}
	}

	desiredWorkers, totalQueueMessages, idleWorkers := GetDesiredWorkersMultiQueue(
		deploymentName,
		qSpecs,
		workerPodAutoScaler.Spec.Queues,
		currentWorkers,
		*workerPodAutoScaler.Spec.MinReplicas,
		*workerPodAutoScaler.Spec.MaxReplicas,
		workerPodAutoScaler.GetMaxDisruption(c.defaultMaxDisruption),
	)
	// set metrics
	for _, qSpec := range qSpecs {
		qMsgs.WithLabelValues(
			name,
			namespace,
			qSpec.Name,
		).Set(float64(qSpec.Messages))
		qMsgsSPM.WithLabelValues(
			name,
			namespace,
			qSpec.Name,
		).Set(qSpec.MessagesSentPerMinute)
	}
	workersIdle.WithLabelValues(
		name,
		namespace,
		deploymentName,
	).Set(float64(idleWorkers))
	workersCurrent.WithLabelValues(
		name,
		namespace,
		deploymentName,
	).Set(float64(currentWorkers))
	workersDesired.WithLabelValues(
		name,
		namespace,
		deploymentName,
	).Set(float64(desiredWorkers))
	workersAvailable.WithLabelValues(
		name,
		namespace,
		deploymentName,
	).Set(float64(availableWorkers))

	lastScaleTime := workerPodAutoScaler.Status.LastScaleTime.DeepCopy()

	op := GetScaleOperation(
		deploymentName,
		desiredWorkers,
		currentWorkers,
		lastScaleTime,
		c.scaleDownDelay,
	)

	if op == ScaleUp || op == ScaleDown {
		if deploymentName != "" {
			c.updateDeployment(
				ctx,
				workerPodAutoScaler.Namespace, deploymentName, &desiredWorkers)
		}

		now := metav1.Now()
		lastScaleTime = &now
	}

	klog.V(2).Infof("%s scaleOp: %v", deploymentName, scaleOpString(op))

	// Finally, we update the status block of the WorkerPodAutoScalerMultiQueue resource to reflect the
	// current state of the world
	updateWorkerPodAutoScalerStatus(
		ctx,
		name,
		namespace,
		c.customclientset,
		desiredWorkers,
		workerPodAutoScaler,
		currentWorkers,
		availableWorkers,
		totalQueueMessages,
		lastScaleTime,
	)

	loopDurationSeconds.WithLabelValues(
		name,
		namespace,
	).Set(time.Since(now).Seconds())
	loopCountSuccess.WithLabelValues(
		name,
		namespace,
	).Inc()

	// TODO: organize and log events
	// c.recorder.Event(workerPodAutoScaler, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

// updateDeployment updates the Deployment with the desired number of replicas
func (c *Controller) updateDeployment(ctx context.Context, namespace string, deploymentName string, replicas *int32) {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of the Deployment before attempting update
		deployment, getErr := c.deploymentLister.Deployments(namespace).Get(deploymentName)
		if errors.IsNotFound(getErr) {
			return fmt.Errorf("deployment %s was not found in namespace %s",
				deploymentName, namespace)
		}
		if getErr != nil {
			klog.Fatalf("Failed to get deployment: %v", getErr)
		}

		deployment.Spec.Replicas = replicas
		_, updateErr := c.kubeclientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		if updateErr != nil {
			klog.Errorf("Failed to update deployment: %v", updateErr)
		}
		return updateErr
	})
	if retryErr != nil {
		klog.Fatalf("Failed to update deployment (retry failed): %v", retryErr)
	}
}

// updateReplicaSet updates the ReplicaSet with the desired number of replicas
func (c *Controller) updateReplicaSet(ctx context.Context, namespace string, replicaSetName string, replicas *int32) {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of the ReplicaSet before attempting update
		replicaSet, getErr := c.replicaSetLister.ReplicaSets(namespace).Get(replicaSetName)
		if errors.IsNotFound(getErr) {
			return fmt.Errorf("ReplicaSet %s was not found in namespace %s",
				replicaSetName, namespace)
		}
		if getErr != nil {
			klog.Fatalf("Failed to get ReplicaSet: %v", getErr)
		}

		replicaSet.Spec.Replicas = replicas
		_, updateErr := c.kubeclientset.AppsV1().ReplicaSets(namespace).Update(ctx, replicaSet, metav1.UpdateOptions{})
		if updateErr != nil {
			klog.Errorf("Failed to update ReplicaSet: %v", updateErr)
		}
		return updateErr
	})
	if retryErr != nil {
		klog.Fatalf("Failed to update ReplicaSet (retry failed): %v", retryErr)
	}
}

// getMaxDisruptableWorkers gets the maximum number of workers that can
// be scaled down in the single scale down activity.
func getMaxDisruptableWorkers(
	maxDisruption *string,
	currentWorkers int32) int32 {

	if maxDisruption == nil {
		klog.Fatalf("maxDisruption default is not being set. Exiting")
	}

	maxDisruptionIntOrStr := intstr.Parse(*maxDisruption)
	maxDisruptableWorkers, err := intstr.GetValueFromIntOrPercent(
		&maxDisruptionIntOrStr, int(currentWorkers), true,
	)

	if err != nil {
		klog.Fatalf("Error calculating maxDisruptable workers, err: %v", err)
	}

	return int32(maxDisruptableWorkers)
}

// getMinWorkers gets the min workers based on the
// velocity metric: messagesSentPerMinute
func getMinWorkers(
	messagesSentPerMinute float64,
	minWorkers int32,
	secondsToProcessOneJob float64) int32 {

	// disable this feature for WPA queues which have not specified
	// processing time
	if secondsToProcessOneJob == 0.0 {
		return minWorkers
	}

	workersBasedOnMessagesSent := int32(math.Ceil((secondsToProcessOneJob * messagesSentPerMinute) / 60))
	klog.V(4).Infof("%v, workersBasedOnMessagesSent=%v\n", secondsToProcessOneJob, workersBasedOnMessagesSent)
	if workersBasedOnMessagesSent > minWorkers {
		return workersBasedOnMessagesSent
	}
	return minWorkers
}

// getMinMultiQueueWorkers gets the min workers based on the
// velocity metric: messagesSentPerMinute
func getMinMultiQueueWorkers(
	deploymentName string,
	queueSpecs map[string]queue.QueueSpec,
	minWorkers int32) int32 {
	var totalMinWorkers int32
	for _, qSpec := range queueSpecs {
		workersBasedOnMessagesSent := int32(math.Ceil((qSpec.SecondsToProcessOneJob * qSpec.MessagesSentPerMinute) / 60))
		klog.V(4).Infof("%s: secondsToProcessOneJob=%v, workersBasedOnMessagesSent=%v\n", qSpec.Name, qSpec.SecondsToProcessOneJob, workersBasedOnMessagesSent)
		totalMinWorkers += workersBasedOnMessagesSent
	}
	klog.V(4).Infof("%s: totalWorkersBasedOnMessagesSent=%v\n", deploymentName, totalMinWorkers)

	if totalMinWorkers > minWorkers {
		return totalMinWorkers
	}
	return minWorkers
}

func isChangeTooSmall(desired int32, current int32, tolerance float64) bool {
	return math.Abs(float64(desired-current))/float64(current) <= tolerance
}

// GetDesiredWorkers finds the desired number of workers which are required
func GetDesiredWorkers(
	queueName string,
	queueMessages int32,
	messagesSentPerMinute float64,
	secondsToProcessOneJob float64,
	targetMessagesPerWorker int32,
	currentWorkers int32,
	idleWorkers int32,
	minWorkers int32,
	maxWorkers int32,
	maxDisruption *string) int32 {

	klog.V(4).Infof("%s min=%v, max=%v, targetBacklog=%v \n",
		queueName, minWorkers, maxWorkers, targetMessagesPerWorker)

	// overwrite the minimum workers needed based on
	// messagesSentPerMinute and secondsToProcessOneJob
	// this feature is disabled if secondsToProcessOneJob is not set or is 0.0
	minWorkers = getMinWorkers(
		messagesSentPerMinute,
		minWorkers,
		secondsToProcessOneJob,
	)

	// gets the maximum number of workers that can be scaled down in a
	// single scale down activity.
	maxDisruptableWorkers := getMaxDisruptableWorkers(
		maxDisruption, currentWorkers,
	)

	tolerance := 0.1
	desiredWorkers := int32(math.Ceil(
		float64(queueMessages) / float64(targetMessagesPerWorker)),
	)

	klog.V(4).Infof("%s qMsgs=%v, qMsgsPerMin=%v \n",
		queueName, queueMessages, messagesSentPerMinute)
	klog.V(4).Infof("%s secToProcessJob=%v, maxDisruption=%v \n",
		queueName, secondsToProcessOneJob, *maxDisruption)
	klog.V(4).Infof("%s current=%v, idle=%v \n",
		queueName, currentWorkers, idleWorkers)
	klog.V(3).Infof("%s minComputed=%v, maxDisruptable=%v\n",
		queueName, minWorkers, maxDisruptableWorkers)

	if currentWorkers == 0 {
		return convertDesiredReplicasWithRules(
			currentWorkers,
			desiredWorkers,
			minWorkers,
			maxWorkers,
			maxDisruptableWorkers,
		)
	}

	if queueMessages > 0 {
		if isChangeTooSmall(desiredWorkers, currentWorkers, tolerance) {
			// desired is same as current in this scenario
			return convertDesiredReplicasWithRules(
				currentWorkers,
				currentWorkers,
				minWorkers,
				maxWorkers,
				maxDisruptableWorkers,
			)
		}

		return convertDesiredReplicasWithRules(
			currentWorkers,
			desiredWorkers,
			minWorkers,
			maxWorkers,
			maxDisruptableWorkers,
		)
	} else if messagesSentPerMinute > 0 && secondsToProcessOneJob > 0.0 {
		// this is the case in which there is no backlog visible.
		// (mostly because the workers picks up jobs very quickly)
		// But the queue has throughput, so we return the minWorkers.
		// Note: minWorkers is updated based on
		// messagesSentPerMinute and secondsToProcessOneJob
		// desried is the minReplicas in this scenario
		return convertDesiredReplicasWithRules(
			currentWorkers,
			minWorkers,
			minWorkers,
			maxWorkers,
			maxDisruptableWorkers,
		)
	}

	// Attempt for massive scale down
	if currentWorkers == idleWorkers {
		desiredWorkers := int32(0)
		// for massive scale down to happen maxDisruptableWorkers
		// should be ignored
		return convertDesiredReplicasWithRules(
			currentWorkers,
			desiredWorkers,
			minWorkers,
			maxWorkers,
			currentWorkers,
		)
	}

	// Attempt partial scale down since there is no backlog or in-processing
	// messages.
	return convertDesiredReplicasWithRules(
		currentWorkers,
		minWorkers,
		minWorkers,
		maxWorkers,
		maxDisruptableWorkers,
	)
}

// GetDesiredWorkersMultiQueue finds the desired number of workers which are required
func GetDesiredWorkersMultiQueue(
	deploymentName string,
	queueSpecs map[string]queue.QueueSpec,
	k8QueueSpecs []v1.Queue,
	currentWorkers int32,
	minWorkers int32,
	maxWorkers int32,
	maxDisruption *string) (int32, int32, int32) {
	for _, k8QSpec := range k8QueueSpecs {
		qSpec := queueSpecs[k8QSpec.URI]
		klog.V(4).Infof("%s min=%v, max=%v, targetBacklog=%v \n",
			qSpec.Name, minWorkers, maxWorkers, k8QSpec.TargetMessagesPerWorker)
	}

	totalQueueMessages, totalMessagesSentPerMinute, idleWorkers := queue.Aggregate(queueSpecs)

	// overwrite the minimum workers needed based on
	// messagesSentPerMinute and secondsToProcessOneJob
	// this feature is disabled if secondsToProcessOneJob is not set or is 0.0
	minWorkers = getMinMultiQueueWorkers(
		deploymentName,
		queueSpecs,
		minWorkers,
	)

	// gets the maximum number of workers that can be scaled down in a
	// single scale down activity.
	maxDisruptableWorkers := getMaxDisruptableWorkers(
		maxDisruption, currentWorkers,
	)

	tolerance := 0.1
	var desiredWorkers int32
	for _, k8QSpec := range k8QueueSpecs {
		qSpec := queueSpecs[k8QSpec.URI]
		desiredWorkers += int32(math.Ceil(
			float64(qSpec.Messages) / float64(k8QSpec.TargetMessagesPerWorker)),
		)
	}
	klog.V(4).Infof("MinWorkers: %v, MaxWorkers: %v, DesiredWorkers: %v, CurrentWorkers: %v, idleWorkers=%v\n", minWorkers, maxWorkers, desiredWorkers, currentWorkers, idleWorkers)

	if currentWorkers == 0 {
		return convertDesiredReplicasWithRules(
			currentWorkers,
			desiredWorkers,
			minWorkers,
			maxWorkers,
			maxDisruptableWorkers,
		), totalQueueMessages, idleWorkers
	}

	if totalQueueMessages > 0 {
		if isChangeTooSmall(desiredWorkers, currentWorkers, tolerance) {
			// desired is same as current in this scenario
			return convertDesiredReplicasWithRules(
				currentWorkers,
				currentWorkers,
				minWorkers,
				maxWorkers,
				maxDisruptableWorkers,
			), totalQueueMessages, idleWorkers
		}

		return convertDesiredReplicasWithRules(
			currentWorkers,
			desiredWorkers,
			minWorkers,
			maxWorkers,
			maxDisruptableWorkers,
		), totalQueueMessages, idleWorkers
	} else if totalMessagesSentPerMinute > 0 {
		// this is the case in which there is no backlog visible.
		// (mostly because the workers picks up jobs very quickly)
		// But the queue has throughput, so we return the minWorkers.
		// Note: minWorkers is updated based on
		// messagesSentPerMinute and secondsToProcessOneJob
		// desried is the minReplicas in this scenario
		return convertDesiredReplicasWithRules(
			currentWorkers,
			minWorkers,
			minWorkers,
			maxWorkers,
			maxDisruptableWorkers,
		), totalQueueMessages, idleWorkers
	}

	// Attempt for massive scale down
	if currentWorkers == idleWorkers {
		desiredWorkers := int32(0)
		// for massive scale down to happen maxDisruptableWorkers
		// should be ignored
		return convertDesiredReplicasWithRules(
			currentWorkers,
			desiredWorkers,
			minWorkers,
			maxWorkers,
			currentWorkers,
		), totalQueueMessages, idleWorkers
	}

	// Attempt partial scale down since there is no backlog or in-processing
	// messages.
	return convertDesiredReplicasWithRules(
		currentWorkers,
		minWorkers,
		minWorkers,
		maxWorkers,
		maxDisruptableWorkers,
	), totalQueueMessages, idleWorkers
}

func convertDesiredReplicasWithRules(
	current int32,
	desired int32,
	min int32,
	max int32,
	maxDisruptable int32) int32 {

	if min >= max {
		return max
	}

	if (current - desired) > maxDisruptable {
		desired = current - maxDisruptable
	}

	if desired > max {
		return max
	}
	if desired < min {
		return min
	}
	return desired
}

func updateWorkerPodAutoScalerStatus(
	ctx context.Context,
	name string,
	namespace string,
	customclientset clientset.Interface,
	desiredWorkers int32,
	workerPodAutoScalerMultiQueue *v1.WorkerPodAutoScalerMultiQueue,
	currentWorkers int32,
	availableWorkers int32,
	queueMessages int32,
	lastScaleTime *metav1.Time) {

	if workerPodAutoScalerMultiQueue.Status.CurrentReplicas == currentWorkers &&
		workerPodAutoScalerMultiQueue.Status.AvailableReplicas == availableWorkers &&
		workerPodAutoScalerMultiQueue.Status.DesiredReplicas == desiredWorkers &&
		workerPodAutoScalerMultiQueue.Status.CurrentMessages == queueMessages &&
		workerPodAutoScalerMultiQueue.Status.LastScaleTime.Equal(lastScaleTime) {
		klog.V(4).Infof("%s/%s: WPA status is already up to date\n", namespace, name)
		return
	} else {
		klog.V(4).Infof("%s/%s: Updating wpa status\n", namespace, name)
	}

	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	workerPodAutoScalerMultiQueueCopy := workerPodAutoScalerMultiQueue.DeepCopy()
	workerPodAutoScalerMultiQueueCopy.Status.CurrentReplicas = currentWorkers
	workerPodAutoScalerMultiQueueCopy.Status.AvailableReplicas = availableWorkers
	workerPodAutoScalerMultiQueueCopy.Status.DesiredReplicas = desiredWorkers
	workerPodAutoScalerMultiQueueCopy.Status.CurrentMessages = queueMessages
	workerPodAutoScalerMultiQueueCopy.Status.LastScaleTime = lastScaleTime
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the WorkerPodAutoScalerMultiQueue resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := customclientset.K8sV1().WorkerPodAutoScalerMultiQueues(workerPodAutoScalerMultiQueue.Namespace).UpdateStatus(ctx, workerPodAutoScalerMultiQueueCopy, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Error updating wpa status, err: %v", err)
		return
	}
	klog.V(4).Infof("%s/%s: Updated wpa status\n", namespace, name)
}

// getKeyForWorkerPodAutoScaler takes a WorkerPodAutoScalerMultiQueue resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than WorkerPodAutoScalerMultiQueue.
func (c *Controller) getKeyForWorkerPodAutoScaler(obj interface{}) string {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return ""
	}
	return key
}

func (c *Controller) enqueueAddWorkerPodAutoScaler(obj interface{}) {
	c.workqueue.Add(WokerPodAutoScalerEvent{
		key:  c.getKeyForWorkerPodAutoScaler(obj),
		name: WokerPodAutoScalerEventAdd,
	})
}

func (c *Controller) enqueueUpdateWorkerPodAutoScaler(obj interface{}) {
	c.workqueue.Add(WokerPodAutoScalerEvent{
		key:  c.getKeyForWorkerPodAutoScaler(obj),
		name: WokerPodAutoScalerEventUpdate,
	})
}

func (c *Controller) enqueueDeleteWorkerPodAutoScaler(obj interface{}) {
	c.workqueue.Add(WokerPodAutoScalerEvent{
		key:  c.getKeyForWorkerPodAutoScaler(obj),
		name: WokerPodAutoScalerEventDelete,
	})
}
