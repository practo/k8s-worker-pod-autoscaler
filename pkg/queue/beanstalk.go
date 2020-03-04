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

	pollInterval int

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
	pollInterval int) (QueuingService, error) {

	return &Beanstalk{
		queues:       queues,
		bsClientPool: make(map[string]*beanstalkd.BeanstalkdClient),

		pollInterval: pollInterval,

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

func getTubeName(uri string) string {
	splitted := strings.Split(uri, "|")
	return splitted[1]
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
	reservedJobs, err := strconv.Atoi(stats["current-jobs-reserved"])
	if err != nil {
		return queueLength, err
	}

	queueLength = int32(delayedJobs + readyJobs + reservedJobs)

	return queueLength, nil
}

func (b *Beanstalk) listAllCache() map[string]float64 {
	cacheResultCh := make(chan map[string]float64)
	b.cacheListCh <- cacheResultCh
	return <-cacheResultCh
}

func (b *Beanstalk) getCache(queueURI string) (float64, bool) {
	allCache := b.listAllCache()
	if cache, ok := allCache[queueURI]; ok {
		return cache, true
	}
	return 0.0, false
}

func (b *Beanstalk) updateCache(key string, cache float64) {
	b.cacheUpdateCh <- map[string]float64{
		key: cache,
	}
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
			klog.Info("Stopping beanstalk syncer gracefully.")
			return
		}
	}
}

func (b *Beanstalk) poll(key string, queueSpec QueueSpec) {
	idleWorkers := 0
	if !strings.Contains(queueSpec.uri, "bs") {

		return
	}
	messagesReceived := int32(-1)
	messagesReceived, err := b.receiveQueueLength(queueSpec.uri)
	if err != nil {
		klog.Errorf("Unable to find queue %q, %v.", queueSpec.name, err)

	}

	if messagesReceived == 0 || messagesReceived < queueSpec.workers {
		idleWorkers = 1
	} else if queueSpec.workers == 0 {
		idleWorkers = 0
	}

	b.queues.updateMessage(key, messagesReceived)
	b.queues.updateIdleWorkers(key, int32(idleWorkers))

	time.Sleep(time.Duration(b.pollInterval) * time.Second)
	return
}
