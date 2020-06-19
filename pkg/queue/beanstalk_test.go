package queue

import (
	"k8s.io/klog"
	"os/exec"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	// _ "github.com/golang/mock/mockgen"
	"github.com/practo/k8s-worker-pod-autoscaler/pkg/signals"
)

var (
	stopCh = signals.SetupSignalHandler()
)

const (
	localBeanstalkHost = "localhost"
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

	poller, err := NewBeanstalk(BeanstalkQueueService, queues, 1, 1)
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

	klog.Info("Running poll and sync.")
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

	klog.Info("Running poll and sync.")
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

	klog.Info("Running poll and sync.")
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

	klog.Info("Running poll and sync.")
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

func runBeanstalkdProcess(
	startCh chan bool, killCh chan bool, doneCh chan bool) {

	cmd := exec.Command("beanstalkd", "-l", "localhost", "-p", "11300")
	if err := cmd.Start(); err != nil {
		klog.Errorf("Error starting the beanstald process: %v\n", err)
		return
	}
	klog.Info("Started local beanstalkd process.")
	startCh <- true
	for {
		switch {
		case <-killCh:
			klog.Info("Killing beanstalkd process.")
			if err := cmd.Process.Kill(); err != nil {
				klog.Errorf("Error killing the beanstalkd process: %v\n", err)
				return
			}
			klog.Info("Killed beanstalkd.")
			doneCh <- true
			return
		}
	}
}

func GetBeanstalkClient(queueURI string) (BeanstalkClientInterface, error) {
	beastalkClient, err := NewBeanstalkClient(queueURI)
	for connect := 0; err != nil && connect < 3; connect++ {
		beastalkClient, err = NewBeanstalkClient(queueURI)
		if err == nil {
			return beastalkClient, nil
		}
		klog.Warningf("Retrying connection to local beanstalk")
		time.Sleep(1 * time.Second)
		continue
	}
	return beastalkClient, err
}

func TestBeanstalkClient(t *testing.T) {
	startCh := make(chan bool)
	killCh := make(chan bool)
	doneCh := make(chan bool)
	go runBeanstalkdProcess(startCh, killCh, doneCh)
	<-startCh

	queueName := "otpsender"
	queueURI := getQueueURI("", queueName)

	beastalkClient, err := GetBeanstalkClient(queueURI)
	if err != nil {
		t.Errorf("Failed to connect to %v:11300", localBeanstalkHost)
		return
	}

	// test1: test when nothing is there what happens
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

	// test2: add a job in the queue
	beastalkClient.put([]byte("51620"), 1, 0, time.Minute)
	jobsWaiting, idleWorkers, jobsReserved, err = beastalkClient.getStats()
	if err != nil {
		klog.Fatalf("Error getting stats(2): %v\n", err)
	}
	if jobsWaiting != 1 || idleWorkers != 0 || jobsReserved != 0 {
		t.Errorf("expected jobsWaiting=1,"+
			"idleWorkers=0,"+
			"jobsReserved=0,"+
			"got=(%v, %v, %v) resp.\n", jobsWaiting, idleWorkers, jobsReserved)
	}

	// test3: add another job in the queue
	_, err = beastalkClient.put([]byte("51621"), 1, 0, time.Minute)
	if err != nil {
		t.Errorf("expected jobs put to work, error happened: %v\n", err)
		return
	}
	jobsWaiting, idleWorkers, jobsReserved, err = beastalkClient.getStats()
	if err != nil {
		klog.Fatalf("Error getting stats(3): %v\n", err)
	}
	if jobsWaiting != 2 || idleWorkers != 0 || jobsReserved != 0 {
		t.Errorf("expected jobsWaiting=2,"+
			"idleWorkers=0,"+
			"jobsReserved=0,"+
			"got=(%v, %v, %v) resp.\n", jobsWaiting, idleWorkers, jobsReserved)
	}

	klog.Info("Sleeping for 30 seconds, please restart")
	time.Sleep(30 * time.Second)
	klog.Info("running long poll")

	// test4: consume 1 job from the queue using long poll and put the job back
	jobsWaiting, idleWorkers, err = beastalkClient.longPollReceiveMessage(
		int64(10),
	)
	if err != nil {
		klog.Fatalf("Error getting stats(4): %v\n", err)
	}
	if jobsWaiting != 1 || idleWorkers != 0 {
		t.Errorf("expected jobsWaiting=1,"+
			"idleWorkers=0,"+
			"got=(%v, %v) resp.\n", jobsWaiting, idleWorkers)
	}

	killCh <- true
	<-doneCh
	klog.Info("Beanstalkd process gracefully shutdown.")

	// test5: testing if restarting beanstalkd, re-establishes the conn
	// in mutliple scenarios

	// test 5a
	go runBeanstalkdProcess(startCh, killCh, doneCh)
	<-startCh
	_, err = GetBeanstalkClient(queueURI)
	if err != nil {
		t.Errorf("Failed to connect to %v:11300", localBeanstalkHost)
		return
	}
	klog.Info("5a> Beanstalkd running again, checking re-establishment")
	jobsWaiting, idleWorkers, err = beastalkClient.longPollReceiveMessage(
		int64(10),
	)
	if err != nil {
		klog.Fatalf("Error doing longPoll(5a): %v\n", err)
	}
	if jobsWaiting != 0 || idleWorkers != 0 {
		t.Errorf("expected jobsWaiting=0,"+
			"idleWorkers=0,"+
			"got=(%v, %v) resp.\n", jobsWaiting, idleWorkers)
	}
	killCh <- true
	<-doneCh
	klog.Info("5a> Beanstalkd process gracefully shutdown.")

	// test 5b
	go runBeanstalkdProcess(startCh, killCh, doneCh)
	<-startCh
	_, err = GetBeanstalkClient(queueURI)
	if err != nil {
		t.Errorf("Failed to connect to %v:11300", localBeanstalkHost)
		return
	}
	klog.Info("5b> Beanstalkd running again, checking re-establishment")
	_, err = beastalkClient.put([]byte("51621"), 1, 0, time.Minute)
	if err != nil {
		t.Errorf("expected jobs put to work, error happened: %v\n", err)
		return
	}
	jobsWaiting, idleWorkers, jobsReserved, err = beastalkClient.getStats()
	if err != nil {
		klog.Fatalf("Error getting stats(1): %v\n", err)
	}
	if jobsWaiting != 1 || idleWorkers != 0 || jobsReserved != 0 {
		t.Errorf("expected jobsWaiting=0,"+
			"idleWorkers=0,"+
			"jobsReserved=0,"+
			"got=(%v, %v, %v) resp.\n", jobsWaiting, idleWorkers, jobsReserved)
	}
	killCh <- true
	<-doneCh
	klog.Info("5b> Beanstalkd process gracefully shutdown.")

	// test 5c
	go runBeanstalkdProcess(startCh, killCh, doneCh)
	<-startCh
	_, err = GetBeanstalkClient(queueURI)
	if err != nil {
		t.Errorf("Failed to connect to %v:11300", localBeanstalkHost)
		return
	}
	klog.Info("5c> Beanstalkd running again, checking re-establishment")
	jobsWaiting, idleWorkers, jobsReserved, err = beastalkClient.getStats()
	if err != nil {
		klog.Fatalf("Error getting stats(1): %v\n", err)
	}
	if jobsWaiting != 0 || idleWorkers != 0 || jobsReserved != 0 {
		t.Errorf("expected jobsWaiting=0,"+
			"idleWorkers=0,"+
			"jobsReserved=0,"+
			"got=(%v, %v, %v) resp.\n", jobsWaiting, idleWorkers, jobsReserved)
	}
	killCh <- true
	<-doneCh
	klog.Info("5c> Beanstalkd process gracefully shutdown.")
}
