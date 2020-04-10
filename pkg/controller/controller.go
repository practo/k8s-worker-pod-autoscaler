package controller

import (
	"fmt"
	"math"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

type WokerPodAutoScalerEvent struct {
	key  string
	name string
}

// Controller is the controller implementation for WorkerPodAutoScaler resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// customclientset is a clientset for our own API group
	customclientset clientset.Interface

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

	switch event.name {
	case WokerPodAutoScalerEventAdd:
		err = c.Queues.Add(
			namespace,
			name,
			workerPodAutoScaler.Spec.QueueURI,
			workers,
			workerPodAutoScaler.Spec.SecondsToProcessOneJob,
		)
	case WokerPodAutoScalerEventUpdate:
		err = c.Queues.Add(
			namespace,
			name,
			workerPodAutoScaler.Spec.QueueURI,
			workers,
			workerPodAutoScaler.Spec.SecondsToProcessOneJob,
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
		queueMessages,
		messagesSentPerMinute,
		workerPodAutoScaler.Spec.SecondsToProcessOneJob,
		*workerPodAutoScaler.Spec.TargetMessagesPerWorker,
		deployment.Status.AvailableReplicas,
		idleWorkers,
		*workerPodAutoScaler.Spec.MinReplicas,
		*workerPodAutoScaler.Spec.MaxReplicas,
		workerPodAutoScaler.Spec.Disruptable,
	)
	klog.Infof("%s: messages: %d, idle: %d, desired: %d", queueName, queueMessages, idleWorkers, desiredWorkers)

	if desiredWorkers != *deployment.Spec.Replicas {
		c.updateDeployment(workerPodAutoScaler.Namespace, deploymentName, &desiredWorkers)
	}

	// Finally, we update the status block of the WorkerPodAutoScaler resource to reflect the
	// current state of the world
	err = c.updateWorkerPodAutoScalerStatus(
		desiredWorkers,
		workerPodAutoScaler,
		deployment.Status.AvailableReplicas,
		queueMessages,
	)
	if err != nil {
		// TODO: till the api server has https://github.com/kubernetes/kubernetes/pull/72856 fix
		// ignore this error
		// this was fixed in 1.13.1 release
		if strings.Contains(err.Error(), "0-length response with status code: 200 and content type:") {
			klog.Errorf("Error updating status of wpa (1.13 apiserver has the fix): %v", err)
			return nil
		}
		klog.Fatalf("Error updating status of wpa: %v", err)
	}

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

// getMinWorkers gets the min workers based on the
// velocity metric: messagesSentPerMinute
func (c *Controller) getMinWorkers(
	messagesSentPerMinute float64,
	minWorkers int32,
	secondsToProcessOneJob float64) int32 {

	// disable this feature for WPA queues which have not specified
	// processing time
	if secondsToProcessOneJob == 0.0 {
		return minWorkers
	}

	workersBasedOnMessagesSent := int32(math.Ceil(messagesSentPerMinute / (secondsToProcessOneJob * 60)))
	if workersBasedOnMessagesSent > minWorkers {
		return workersBasedOnMessagesSent
	}
	return minWorkers
}

// getDesiredWorkers finds the desired number of workers which are required
// test case run: https://play.golang.org/p/_dFbbhb1J_8
func (c *Controller) getDesiredWorkers(
	queueMessages int32,
	messagesSentPerMinute float64,
	secondsToProcessOneJob float64,
	targetMessagesPerWorker int32,
	currentWorkers int32,
	idleWorkers int32,
	minWorkers int32,
	maxWorkers int32,
	disruptable bool) int32 {

	// overwrite the minimum workers needed based on
	// messagesSentPerMinute and secondsToProcessOneJob
	// this feature is disabled if secondsToProcessOneJob is not set or is 0.0
	minWorkers = c.getMinWorkers(messagesSentPerMinute, minWorkers, secondsToProcessOneJob)

	tolerance := 0.1
	usageRatio := float64(queueMessages) / float64(targetMessagesPerWorker)
	if currentWorkers == 0 {
		desiredWorkers := int32(math.Ceil(usageRatio))
		return convertDesiredReplicasWithRules(desiredWorkers, minWorkers, maxWorkers)
	}

	if queueMessages > 0 {
		// return the current replicas if the change would be too small
		if math.Abs(1.0-usageRatio) <= tolerance {
			return currentWorkers
		}

		if queueMessages < targetMessagesPerWorker {
			return currentWorkers
		}

		desiredWorkers := int32(math.Ceil(usageRatio * float64(currentWorkers)))
		if !disruptable && desiredWorkers < currentWorkers {
			return currentWorkers
		}
		return convertDesiredReplicasWithRules(desiredWorkers, minWorkers, maxWorkers)
	} else if messagesSentPerMinute > 0 && secondsToProcessOneJob > 0.0 {
		// this is the case in which there is no backlog visible.
		// (mostly because the workers picks up jobs very quickly)
		// But the queue has throughput, so we return the minWorkers.
		// Note: minWorkers is updated based on
		// messagesSentPerMinute and secondsToProcessOneJob
		return minWorkers
	}

	// Attempt for massive scale down
	if idleWorkers > 0 {
		desiredWorkers := currentWorkers - idleWorkers
		return convertDesiredReplicasWithRules(desiredWorkers, minWorkers, maxWorkers)
	}

	return currentWorkers
}

func convertDesiredReplicasWithRules(desired int32, min int32, max int32) int32 {
	if desired > max {
		return max
	}
	if desired < min {
		return min
	}
	return desired
}

func (c *Controller) updateWorkerPodAutoScalerStatus(
	desiredWorkers int32,
	workerPodAutoScaler *v1alpha1.WorkerPodAutoScaler,
	availableReplicas int32,
	queueMessages int32) error {

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
	_, err := c.customclientset.K8sV1alpha1().WorkerPodAutoScalers(workerPodAutoScaler.Namespace).Update(workerPodAutoScalerCopy)
	return err
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
