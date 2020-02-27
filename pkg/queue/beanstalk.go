package queue

import (
	"strconv"
	"strings"
	"time"

	"github.com/mpdroog/beanstalkd"
	"k8s.io/klog"
)

// Beanstalk is used to by the Poller to get the queue
// information from Beanstalk, it implements the QueuingService interface
type Beanstalk struct {
	queues       *Queues
	bsClientPool map[string]*beanstalkd.BeanstalkdClient

	shortPollInterval time.Duration
	longPollInterval  int64

	// cache the numberOfEmptyReceives as it is refreshed
	// in aws every 5minutes - save un-necessary api calls
	cache               map[string]float64
	cacheValidity       time.Duration
	cacheListCh         chan chan map[string]float64
	cacheUpdateCh       chan map[string]float64
	lastCachedTimestamp int64
}

func NewBeanstalk(
	queues *Queues,
	shortPollInterval int,
	longPollInterval int) (QueuingService, error) {

	return &Beanstalk{
		queues:       queues,
		bsClientPool: make(map[string]*beanstalkd.BeanstalkdClient),

		shortPollInterval: time.Second * time.Duration(shortPollInterval),
		longPollInterval:  int64(longPollInterval),

		cache:         make(map[string]float64),
		cacheValidity: time.Second * time.Duration(300),
		cacheListCh:   make(chan chan map[string]float64),
		cacheUpdateCh: make(chan map[string]float64),
	}, nil
}

func (b *Beanstalk) getBeanstalkClient(queueURI string) (*beanstalkd.BeanstalkdClient, error) {
	if b.bsClientPool[queueURI] == nil {
		bsConn, err := beanstalkd.Dial(getHostFromURI(queueURI))
		if err != nil {
			return nil, err
		}
		b.bsClientPool[queueURI] = bsConn
	}
	return b.bsClientPool[queueURI], nil
}

func getTubeName(queueURI string) string {
	return "test"
}

func getHostFromURI(uri string) string {
	splitted := strings.Split(uri, "|")
	return splitted[2] + ":" + splitted[3]
}

func (b *Beanstalk) receiveQueueLength(queueURI string) (int32, error) {
	queueLength := int32(0)
	bsConn, err := b.getBeanstalkClient(queueURI)
	if err != nil {
		return queueLength, err
	}
	stats, err := bsConn.StatsTube(getTubeName(queueURI))
	if err != nil {
		return queueLength, err
	}
	readyJobs, err := strconv.Atoi(stats["current-jobs-ready"])
	if err != nil {
		return queueLength, err
	}
	delayedJobs, err := strconv.Atoi(stats["current-jobs-delayed"])
	if err != nil {
		return queueLength, err
	}

	queueLength = int32(delayedJobs + readyJobs)

	return queueLength, nil
}

func (b *Beanstalk) Sync(stopCh <-chan struct{}) {

	for {
		select {
		case update := <-b.cacheUpdateCh:
			for key, value := range update {
				b.cache[key] = value
			}
		case cacheResultCh := <-b.cacheListCh:
			cacheResultCh <- b.cache
		case <-stopCh:
			klog.Info("Stopping sqs syncer gracefully.")
			return
		}
	}
}

func (b *Beanstalk) poll(key string, queueSpec QueueSpec) {
	if !strings.Contains(queueSpec.uri, "bs") {
		return
	}
	messagesReceived := int32(-1)
	messagesReceived, err := b.receiveQueueLength(queueSpec.uri)
	if err != nil {
		klog.Errorf("Unable to find queue %q, %v.", queueSpec.name, err)
	}

	b.queues.updateMessage(key, messagesReceived)
	b.queues.updateIdleWorkers(key, messagesReceived)
	klog.Infof("Waitiing for 3 sec: ", key, messagesReceived)
	time.Sleep(3 * time.Second)
	return
}
