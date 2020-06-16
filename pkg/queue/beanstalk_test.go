package queue

import (
	"k8s.io/klog"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	// _ "github.com/golang/mock/mockgen"
	"github.com/beanstalkd/go-beanstalk"
	"github.com/practo/k8s-worker-pod-autoscaler/pkg/signals"
)

var (
	stopCh = signals.SetupSignalHandler()
)

const (
	localBeanstalkHost = "host.docker.internal"
)

func init() {
	klog.InitFlags(nil)
}

func getQueueURI(namespace string, name string) string {
	var host string
	if namespace != "" {
		host = "beanstalkd" + "." + namespace
	} else {
		host = localBeanstalkHost
	}
	return "beanstalk://" + host + ":11300/" + name
}

func buildQueues(
	doneChan chan struct{},
	queueSpecs []QueueSpec) (*Queues, *Beanstalk, error) {

	queues := NewQueues()
	go queues.Sync(stopCh)
	for _, spec := range queueSpecs {
		queues.Add(
			spec.namespace,
			spec.name,
			getQueueURI(spec.namespace, spec.name),
			spec.workers,
			spec.secondsToProcessOneJob,
		)
		<-doneChan
	}

	poller, err := NewBeanstalk(queues, 1, 1)
	if err != nil {
		return nil, nil, err
	}
	go poller.Sync(stopCh)

	return queues, poller.(*Beanstalk), nil
}

func TestPollSyncWhenNoMessagesInQueue(t *testing.T) {
	doneChan := make(chan struct{}, 1)
	doneQueueSync = func() {
		doneChan <- struct{}{}
	}
	defer func() {
		doneQueueSync = func() {}
	}()

	queueSpecs := []QueueSpec{
		QueueSpec{
			name:                   "otpsender",
			namespace:              "testns",
			workers:                0,
			secondsToProcessOneJob: 0.0,
		},
	}
	messages := int32(0)
	name := queueSpecs[0].name
	namespace := queueSpecs[0].namespace
	queueURI := getQueueURI(namespace, name)
	queues, poller, err := buildQueues(doneChan, queueSpecs)

	if err != nil {
		klog.Fatalf("error setting up beanstalk test: %v\n", err)
	}
	key := getKey(namespace, name)
	queues.updateMessage(key, messages)
	<-doneChan

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBeanstalkClient := NewMockBeanstalkClientInterface(mockCtrl)
	mockBeanstalkClient.
		EXPECT().
		longPollReceiveMessage(int64(1)).
		Return(int32(0), messages, nil).
		Times(1)

	// TODO: due to this call not able to use poller as an interface
	poller.clientPool.Store(queueURI, mockBeanstalkClient)

	klog.Info("Running poll and sync")
	poller.poll(key, queues.item[key])
	<-doneChan
	<-doneChan

	nameGot, messagesGot, messagesPerMinGot, idleGot := queues.GetQueueInfo(
		namespace, name,
	)

	if name != nameGot {
		t.Errorf("expected %s qName, got=%v\n", name, nameGot)
	}

	if messagesGot != messages {
		t.Errorf("expected 0 messages, got=%v\n", messages)
	}

	if messagesPerMinGot != -1 {
		t.Errorf(
			"expected -1 messagesSentPerMinute, got=%v\n", messagesPerMinGot)
	}

	if idleGot != 0 {
		t.Errorf("expected 0 idle, got=%v\n", idleGot)
	}
}

func TestPollSyncWhenMessagesInQueue(t *testing.T) {
	doneChan := make(chan struct{}, 1)
	doneQueueSync = func() {
		doneChan <- struct{}{}
	}
	defer func() {
		doneQueueSync = func() {}
	}()

	queueSpecs := []QueueSpec{
		QueueSpec{
			name:                   "otpsender",
			namespace:              "testns",
			workers:                0,
			secondsToProcessOneJob: 0.0,
		},
	}
	messages := int32(25)
	name := queueSpecs[0].name
	namespace := queueSpecs[0].namespace
	queueURI := getQueueURI(namespace, name)
	queues, poller, err := buildQueues(doneChan, queueSpecs)

	if err != nil {
		klog.Fatalf("error setting up beanstalk test: %v\n", err)
	}
	key := getKey(namespace, name)
	queues.updateMessage(key, messages)
	<-doneChan

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBeanstalkClient := NewMockBeanstalkClientInterface(mockCtrl)
	mockBeanstalkClient.
		EXPECT().
		getStats().
		Return(messages, int32(0), int32(0), nil).
		Times(1)

	// TODO: due to this call not able to use poller as an interface
	poller.clientPool.Store(queueURI, mockBeanstalkClient)

	klog.Info("Running poll and sync")
	poller.poll(key, queues.item[key])
	<-doneChan
	<-doneChan

	nameGot, messagesGot, messagesPerMinGot, idleGot := queues.GetQueueInfo(
		namespace, name,
	)

	if name != nameGot {
		t.Errorf("expected %s qName, got=%v\n", name, nameGot)
	}

	if messagesGot != messages {
		t.Errorf("expected 0 messages, got=%v\n", messages)
	}

	if messagesPerMinGot != -1 {
		t.Errorf(
			"expected -1 messagesSentPerMinute, got=%v\n", messagesPerMinGot)
	}

	if idleGot != -1 {
		t.Errorf("expected 0 idle, got=%v\n", idleGot)
	}
}

func TestPollSyncWhenNoMessagesInQueueButMessagesAreInFlight(t *testing.T) {
	doneChan := make(chan struct{}, 1)
	doneQueueSync = func() {
		doneChan <- struct{}{}
	}
	defer func() {
		doneQueueSync = func() {}
	}()

	queueSpecs := []QueueSpec{
		QueueSpec{
			name:                   "otpsender",
			namespace:              "testns",
			workers:                10,
			secondsToProcessOneJob: 0.0,
		},
	}
	messages := int32(0)
	reserved := int32(5)
	name := queueSpecs[0].name
	namespace := queueSpecs[0].namespace
	queueURI := getQueueURI(namespace, name)
	queues, poller, err := buildQueues(doneChan, queueSpecs)

	if err != nil {
		klog.Fatalf("error setting up beanstalk test: %v\n", err)
	}
	key := getKey(namespace, name)
	queues.updateMessage(key, messages)
	<-doneChan

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBeanstalkClient := NewMockBeanstalkClientInterface(mockCtrl)
	mockBeanstalkClient.
		EXPECT().
		getStats().
		Return(messages, int32(0), reserved, nil).
		Times(2)

	// TODO: due to this call not able to use poller as an interface
	poller.clientPool.Store(queueURI, mockBeanstalkClient)

	klog.Info("Running poll and sync")
	poller.poll(key, queues.item[key])
	<-doneChan

	nameGot, messagesGot, messagesPerMinGot, idleGot := queues.GetQueueInfo(
		namespace, name,
	)

	if name != nameGot {
		t.Errorf("expected %s qName, got=%v\n", name, nameGot)
	}

	if messagesGot != messages {
		t.Errorf("expected 0 messages, got=%v\n", messages)
	}

	if messagesPerMinGot != -1 {
		t.Errorf(
			"expected -1 messagesSentPerMinute, got=%v\n", messagesPerMinGot)
	}

	if idleGot != -1 {
		t.Errorf("expected 0 idle, got=%v\n", idleGot)
	}
}

func TestPollSyncWhenNoMessagesInQueueAndNoMessagesAreInFlight(t *testing.T) {
	doneChan := make(chan struct{}, 1)
	doneQueueSync = func() {
		doneChan <- struct{}{}
	}
	defer func() {
		doneQueueSync = func() {}
	}()

	queueSpecs := []QueueSpec{
		QueueSpec{
			name:                   "otpsender",
			namespace:              "testns",
			workers:                10,
			secondsToProcessOneJob: 0.0,
		},
	}
	messages := int32(0)
	reserved := int32(0)
	name := queueSpecs[0].name
	namespace := queueSpecs[0].namespace
	currentWorkers := int32(queueSpecs[0].workers)
	queueURI := getQueueURI(namespace, name)
	queues, poller, err := buildQueues(doneChan, queueSpecs)

	if err != nil {
		klog.Fatalf("error setting up beanstalk test: %v\n", err)
	}
	key := getKey(namespace, name)
	queues.updateMessage(key, messages)
	<-doneChan

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBeanstalkClient := NewMockBeanstalkClientInterface(mockCtrl)
	mockBeanstalkClient.
		EXPECT().
		getStats().
		Return(messages, currentWorkers, reserved, nil).
		Times(3)

	// TODO: due to this call not able to use poller as an interface
	poller.clientPool.Store(queueURI, mockBeanstalkClient)

	klog.Info("Running poll and sync")
	poller.poll(key, queues.item[key])
	<-doneChan
	<-doneChan

	nameGot, messagesGot, messagesPerMinGot, idleGot := queues.GetQueueInfo(
		namespace, name,
	)

	if name != nameGot {
		t.Errorf("expected %s qName, got=%v\n", name, nameGot)
	}

	if messagesGot != messages {
		t.Errorf("expected 0 messages, got=%v\n", messages)
	}

	if messagesPerMinGot != -1 {
		t.Errorf(
			"expected -1 messagesSentPerMinute, got=%v\n", messagesPerMinGot)
	}

	if idleGot != currentWorkers {
		t.Errorf("expected 0 idle, got=%v\n", idleGot)
	}
}

func TestBeanstalkClientGetStats(t *testing.T) {
	queueName := "otpsender"
	queueURI := getQueueURI("", queueName)
	beastalkClient, err := NewBeanstalkClient(queueURI)
	e, ok := err.(beanstalk.ConnError)
	if ok && e.Err != beanstalk.ErrNotFound {
		t.Skipf("Skipping, connection %v:11300 failed", localBeanstalkHost)
	}

	klog.Info("Note: Testing locally require beanstalkd restart " +
		" in every `make test` invocation. Tests fail if this is not done." +
		" MAC Users: `brew services restart beanstalkd && make test`")

	jobsWaiting, idleWorkers, jobsReserved, err := beastalkClient.getStats()
	if err != nil {
		klog.Fatalf("Error getting stats(1): %v\n", err)
	}
	if jobsWaiting != 0 || idleWorkers != 0 || jobsReserved != 0 {
		t.Errorf("expected jobsWaiting=0,"+
			"idleWorkers=0,"+
			"jobsReserved=0,"+
			"got=(%v, %v, %v) resp.\n", jobsWaiting, idleWorkers, jobsReserved)
	}

	beastalkClient.put([]byte("51620"), 1, 0, time.Minute)
	jobsWaiting, idleWorkers, jobsReserved, err = beastalkClient.getStats()
	if err != nil {
		klog.Fatalf("Error getting stats(2): %v\n", err)
	}
	if jobsWaiting != 1 || idleWorkers != 0 || jobsReserved != 0 {
		t.Errorf("expected jobsWaiting=0,"+
			"idleWorkers=0,"+
			"jobsReserved=0,"+
			"got=(%v, %v, %v) resp.\n", jobsWaiting, idleWorkers, jobsReserved)
	}

	beastalkClient.put([]byte("51621"), 1, 0, time.Minute)
	jobsWaiting, idleWorkers, jobsReserved, err = beastalkClient.getStats()
	if err != nil {
		klog.Fatalf("Error getting stats(2): %v\n", err)
	}
	if jobsWaiting != 2 || idleWorkers != 0 || jobsReserved != 0 {
		t.Errorf("expected jobsWaiting=0,"+
			"idleWorkers=0,"+
			"jobsReserved=0,"+
			"got=(%v, %v, %v) resp.\n", jobsWaiting, idleWorkers, jobsReserved)
	}
}
