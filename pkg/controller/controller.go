package controller

import (
	"fmt"
	"math"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	"k8s.io/klog"

	v1alpha1 "github.com/practo/k8s-worker-pod-autoscaler/pkg/apis/workerpodautoscaler/v1alpha1"
	clientset "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/clientset/versioned"
	samplescheme "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/clientset/versioned/scheme"
	informers "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/informers/externalversions/workerpodautoscaler/v1alpha1"
	listers "github.com/practo/k8s-worker-pod-autoscaler/pkg/generated/listers/workerpodautoscaler/v1alpha1"
	queue "github.com/practo/k8s-worker-pod-autoscaler/pkg/queue"
)

const controllerAgentName = "workerpodautoscaler-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a WorkerPodAutoScaler is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a WorkerPodAutoScaler fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by WorkerPodAutoScaler"
	// MessageResourceSynced is the message used for an Event fired when a WorkerPodAutoScaler
	// is synced successfully
	MessageResourceSynced = "WorkerPodAutoScaler synced successfully"

	// WokerPodAutoScalerEventAdd stores the add event name
	WokerPodAutoScalerEventAdd = "add"

	// WokerPodAutoScalerEventUpdate stores the add event name
	WokerPodAutoScalerEventUpdate = "update"

	// WokerPodAutoScalerEventDelete stores the add event name
	WokerPodAutoScalerEventDelete = "delete"
)

var (
	loopDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wpa",
			Subsystem: "controller",
			Name:      "loop_duration_seconds",
			Help:      "Number of seconds to complete the control loop succesfully, partitioned by wpa name and namespace",
		},
		[]string{"workerpodautoscaler", "namespace"},
	)

	loopCountSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "wpa",
			Subsystem: "controller",
			Name:      "loop_count_success",
			Help:      "How many times the control loop executed succesfully, partitioned by wpa name and namespace",
		},
		[]string{"workerpodautoscaler", "namespace"},
	)
)

func init() {
	prometheus.MustRegister(loopDurationSeconds)
	prometheus.MustRegister(loopCountSuccess)
}

type WokerPodAutoScalerEvent struct {
	key  string
	name string
}

// Controller is the controller implementation for WorkerPodAutoScaler resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// customclientset is a clientset for our own API group
	customclientset            clientset.Interface
	deploymentsLister          appslisters.DeploymentLister
	deploymentsSynced          cache.InformerSynced
	workerPodAutoScalersLister listers.WorkerPodAutoScalerLister
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
	Queues *queue.Queues
}

// NewController returns a new sample controller
func NewController(
	kubeclientset kubernetes.Interface,
	customclientset clientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	workerPodAutoScalerInformer informers.WorkerPodAutoScalerInformer,
	defaultMaxDisruption string,
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
		kubeclientset:              kubeclientset,
		customclientset:            customclientset,
		deploymentsLister:          deploymentInformer.Lister(),
		deploymentsSynced:          deploymentInformer.Informer().HasSynced,
		workerPodAutoScalersLister: workerPodAutoScalerInformer.Lister(),
		workerPodAutoScalersSynced: workerPodAutoScalerInformer.Informer().HasSynced,
		workqueue:                  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "WorkerPodAutoScalers"),
		recorder:                   recorder,
		defaultMaxDisruption:       defaultMaxDisruption,
		Queues:                     queues,
	}

	klog.Info("Setting up event handlers")

	// Set up an event handler for when WorkerPodAutoScaler resources change
	workerPodAutoScalerInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueAddWorkerPodAutoScaler,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueUpdateWorkerPodAutoScaler(new)
		},
		DeleteFunc: controller.enqueueDeleteWorkerPodAutoScaler,
	})
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
	klog.Info("Starting WorkerPodAutoScaler controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.deploymentsSynced, c.workerPodAutoScalersSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch two workers to process WorkerPodAutoScaler resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
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
		// WorkerPodAutoScaler resource to be synced.
		if err := c.syncHandler(event); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(event)
			return fmt.Errorf("error syncing '%s': %s, requeuing", event, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		// klog.Infof("Successfully synced '%s'", event)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the WorkerPodAutoScaler resource
// with the current status of the resource.
func (c *Controller) syncHandler(event WokerPodAutoScalerEvent) error {
	now := time.Now()
	key := event.key
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the WorkerPodAutoScaler resource with this namespace/name
	workerPodAutoScaler, err := c.workerPodAutoScalersLister.WorkerPodAutoScalers(namespace).Get(name)
	if err != nil {
		// The WorkerPodAutoScaler resource may no longer exist, in which case we stop processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("workerPodAutoScaler '%s' in work queue no longer exists", key))
			c.Queues.Delete(namespace, name)
			return nil
		}
		return err
	}

	deploymentName := workerPodAutoScaler.Spec.DeploymentName
	if deploymentName == "" {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		utilruntime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
		return nil
	}

	// Get the deployment with the name specified in WorkerPodAutoScaler.spec
	deployment, err := c.deploymentsLister.Deployments(workerPodAutoScaler.Namespace).Get(deploymentName)
	if errors.IsNotFound(err) {
		return fmt.Errorf("Deployment %s not found in namespace %s",
			deploymentName, workerPodAutoScaler.Namespace)
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	workers := *deployment.Spec.Replicas

	var secondsToProcessOneJob float64
	if workerPodAutoScaler.Spec.SecondsToProcessOneJob != nil {
		secondsToProcessOneJob = *workerPodAutoScaler.Spec.SecondsToProcessOneJob
	}

	switch event.name {
	case WokerPodAutoScalerEventAdd:
		err = c.Queues.Add(
			namespace,
			name,
			workerPodAutoScaler.Spec.QueueURI,
			workers,
			secondsToProcessOneJob,
		)
	case WokerPodAutoScalerEventUpdate:
		err = c.Queues.Add(
			namespace,
			name,
			workerPodAutoScaler.Spec.QueueURI,
			workers,
			secondsToProcessOneJob,
		)
	case WokerPodAutoScalerEventDelete:
		err = c.Queues.Delete(namespace, name)
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to sync queue: %s", err.Error()))
		return err
	}

	queueName, queueMessages, messagesSentPerMinute, idleWorkers := c.Queues.GetQueueInfo(
		namespace, name)
	if queueName == "" {
		return nil
	}

	desiredWorkers := c.getDesiredWorkers(
		queueName,
		queueMessages,
		messagesSentPerMinute,
		secondsToProcessOneJob,
		*workerPodAutoScaler.Spec.TargetMessagesPerWorker,
		deployment.Status.AvailableReplicas,
		idleWorkers,
		*workerPodAutoScaler.Spec.MinReplicas,
		*workerPodAutoScaler.Spec.MaxReplicas,
		workerPodAutoScaler.GetMaxDisruption(c.defaultMaxDisruption),
	)
	klog.Infof("%s: messages: %d, idle: %d, desired: %d", queueName, queueMessages, idleWorkers, desiredWorkers)

	if desiredWorkers != *deployment.Spec.Replicas {
		c.updateDeployment(workerPodAutoScaler.Namespace, deploymentName, &desiredWorkers)
	}

	// Finally, we update the status block of the WorkerPodAutoScaler resource to reflect the
	// current state of the world
	updateWorkerPodAutoScalerStatus(
		name,
		namespace,
		c.customclientset,
		desiredWorkers,
		workerPodAutoScaler,
		deployment.Status.AvailableReplicas,
		queueMessages,
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

// updateDeployment updates the deployment with the desired number of replicas
func (c *Controller) updateDeployment(namespace string, deploymentName string, replicas *int32) {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		deployment, getErr := c.deploymentsLister.Deployments(namespace).Get(deploymentName)
		if errors.IsNotFound(getErr) {
			return fmt.Errorf("Deployment %s was not found in namespace %s",
				deploymentName, namespace)
		}
		if getErr != nil {
			klog.Fatalf("Failed to get deployment: %v", getErr)
		}

		deployment.Spec.Replicas = replicas
		deployment, updateErr := c.kubeclientset.AppsV1().Deployments(namespace).Update(deployment)
		if updateErr != nil {
			klog.Errorf("Failed to update deployment: %v", updateErr)
		}
		return updateErr
	})
	if retryErr != nil {
		klog.Fatalf("Failed to update deployment (retry failed): %v", retryErr)
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
	//klog.Infof("%v, workersBasedOnMessagesSent=%v\n", secondsToProcessOneJob, workersBasedOnMessagesSent)
	if workersBasedOnMessagesSent > minWorkers {
		return workersBasedOnMessagesSent
	}
	return minWorkers
}

// getDesiredWorkers finds the desired number of workers which are required
// test case run: https://play.golang.org/p/_dFbbhb1J_8
func (c *Controller) getDesiredWorkers(
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
	klog.Infof("%s minWorkers=%v, maxDisruptableWorkers=%v\n", queueName, minWorkers, maxDisruptableWorkers)

	tolerance := 0.1
	usageRatio := float64(queueMessages) / float64(targetMessagesPerWorker)
	if currentWorkers == 0 {
		desiredWorkers := int32(math.Ceil(usageRatio))
		return convertDesiredReplicasWithRules(
			currentWorkers,
			desiredWorkers,
			minWorkers,
			maxWorkers,
			maxDisruptableWorkers,
		)
	}

	if queueMessages > 0 {
		// return the current replicas if the change would be too small
		if (math.Abs(1.0-usageRatio) <= tolerance) || (queueMessages < targetMessagesPerWorker) {
			// desired is same as current in this scenario
			return convertDesiredReplicasWithRules(
				currentWorkers,
				currentWorkers,
				minWorkers,
				maxWorkers,
				maxDisruptableWorkers,
			)
		}

		desiredWorkers := int32(math.Ceil(usageRatio * float64(currentWorkers)))
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

	// Attempt to do nothing, desired is same as current
	return convertDesiredReplicasWithRules(
		currentWorkers,
		currentWorkers,
		minWorkers,
		maxWorkers,
		maxDisruptableWorkers,
	)
}

func convertDesiredReplicasWithRules(
	current int32,
	desired int32,
	min int32,
	max int32,
	maxDisruptable int32) int32 {

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
	name string,
	namespace string,
	customclientset clientset.Interface,
	desiredWorkers int32,
	workerPodAutoScaler *v1alpha1.WorkerPodAutoScaler,
	availableReplicas int32,
	queueMessages int32) {

	if workerPodAutoScaler.Status.CurrentReplicas == availableReplicas &&
		workerPodAutoScaler.Status.DesiredReplicas == desiredWorkers &&
		workerPodAutoScaler.Status.CurrentMessages == queueMessages {
		klog.Infof("%s/%s: WPA status is already up to date\n", namespace, name)
		return
	} else {
		klog.Infof("%s/%s: Updating wpa status\n", namespace, name)
	}

	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	workerPodAutoScalerCopy := workerPodAutoScaler.DeepCopy()
	workerPodAutoScalerCopy.Status.CurrentReplicas = availableReplicas
	workerPodAutoScalerCopy.Status.DesiredReplicas = desiredWorkers
	workerPodAutoScalerCopy.Status.CurrentMessages = queueMessages
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the WorkerPodAutoScaler resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := customclientset.K8sV1alpha1().WorkerPodAutoScalers(workerPodAutoScaler.Namespace).Update(workerPodAutoScalerCopy)
	if err != nil {
		klog.Errorf("Error updating wpa status, err: %v", err)
		return
	}
	klog.Infof("%s/%s: Updated wpa status\n", namespace, name)
	return
}

// getKeyForWorkerPodAutoScaler takes a WorkerPodAutoScaler resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than WorkerPodAutoScaler.
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
