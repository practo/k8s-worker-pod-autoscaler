package controller
//
// import (
//     "k8s.io/klog"
//
//     "k8s.io/client-go/kubernetes"
//     "k8s.io/client-go/tools/cache"
//     "k8s.io/client-go/tools/record"
// 	"k8s.io/client-go/util/workqueue"
//     "k8s.io/client-go/kubernetes/scheme"
// )
//
// const controllerAgentName = "workerpodautoscaler-controller"
//
// // Controller is the controller implementation for workerpodautoscaler resources
// type Controller struct {
// 	// kubeclientset is a standard kubernetes clientset
// 	kubeclientset kubernetes.Interface
// 	// customclientset is a clientset for our own API group
// 	customclientset clientset.Interface
//
// 	workerpodautoscalersLister        listers.workerpodautoscalerLister
// 	workerpodautoscalersSynced        cache.InformerSynced
//
// 	// workqueue is a rate limited work queue. This is used to queue work to be
// 	// processed instead of performing it as soon as a change happens. This
// 	// means we can ensure we only process a fixed amount of resources at a
// 	// time, and makes it easy to ensure we are never processing the same item
// 	// simultaneously in two different workers.
// 	workqueue workqueue.RateLimitingInterface
// 	// recorder is an event recorder for recording Event resources to the
// 	// Kubernetes API.
// 	recorder record.EventRecorder
// }
//
// // NewController returns a new workerpodautoscaler controller
// func NewController(
// 	kubeclientset kubernetes.Interface,
// 	customclientset clientset.Interface,
// 	workerpodautoscalerInformer informers.workerpodautoscalerInformer) *Controller {
//
// 	// Create event broadcaster
// 	// Add workerpodautoscaler controller types to the default Kubernetes Scheme so Events can be
// 	// logged for workerpodautoscaler controller types.
// 	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
// 	glog.V(4).Info("Creating event broadcaster")
// 	eventBroadcaster := record.NewBroadcaster()
// 	eventBroadcaster.StartLogging(glog.Infof)
// 	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
// 	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})
//
// 	controller := &Controller{
// 		kubeclientset:                  kubeclientset,
// 		customclientset:                customclientset,
// 		workerpodautoscalersLister:        workerpodautoscalerInformer.Lister(),
// 		workerpodautoscalersSynced:        workerpodautoscalerInformer.Informer().HasSynced,
// 		workqueue:                      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "workerpodautoscalers"),
// 		recorder:                       recorder,
// 	}
//
// 	glog.Info("Setting up event handlers")
// 	// Set up an event handler for when Foo resources change
// 	workerpodautoscalerInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
// 		AddFunc: controller.enqueueFoo,
// 		UpdateFunc: func(old, new interface{}) {
// 			controller.enqueueFoo(new)
// 		},
// 	})
// 	return controller
// }
//
//
// func CreateRunController() {
//
// 	// // clientConfig, err := utils.CreateClientConfig("")
// 	// clientConfig, err := utils.CreateClientConfig("/Users/alok87/.kube/config")
// 	// if err != nil {
// 	// 	glog.Fatalf("Failed to create client config: %v", err)
// 	// }
// 	//
// 	// apiExtensionClient, err := utils.CreateAPIExtensionsClient(clientConfig)
// 	// if err != nil {
// 	// 	glog.Fatalf("Failed creating apiextensions client: %v", err)
// 	// }
// 	//
// 	// err = v1alpha1.CreateCRD(apiExtensionClient)
// 	// if err != nil {
// 	// 	glog.Fatalf("Failed to create CRD: %v", err)
// 	// }
// 	//
// 	// glog.Infof("Waiting 5 seconds for the CRD to be created before we use it")
// 	// time.Sleep(5 * time.Second)
// 	//
// 	// // TOOD: client is not being used, so _
// 	// _, err = v1alpha1.NewClient(clientConfig)
// 	// if err != nil {
// 	// 	glog.Fatalf("Failed to create CRD client: %v", err)
// 	// }
//
// }
