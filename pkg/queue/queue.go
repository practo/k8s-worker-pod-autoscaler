package queue

import (
	"github.com/pkg/errors"
	"math"
	"net/url"
	"strings"

	"github.com/practo/klog/v2"

	statsig "github.com/statsig-io/go-sdk"
)

var (
	// doneQueueSync is a noop function to make synchronization
	// work in unit tests
	doneQueueSync = func() {}
)

const (
	BenanstalkProtocol            = "beanstalk"
	UnsyncedQueueMessageCount     = -1
	UnsyncedMessagesSentPerMinute = -1
	UnsyncedIdleWorkers           = -1
	// PausedQueuesDynamicConfig is the name of statsig dynamic config for paused queues
	PausedQueuesDynamicConfig = "platform-shoryuken-paused-queues"
)

// Queues maintains a list of all queues as specified in WPAs in memory
// The list is kept in sync with the wpa objects
type Queues struct {
	addCh               chan map[string]QueueSpec
	deleteCh            chan string
	listCh              chan chan map[string]QueueSpec
	updateMessageCh     chan map[string]int32
	idleWorkerCh        chan map[string]int32
	updateMessageSentCh chan map[string]float64
	item                map[string]QueueSpec
}

// QueueSpec is the specification for a single queue
type QueueSpec struct {
	Name             string
	namespace        string
	uri              string
	host             string
	protocol         string
	queueServiceName string
	// messages is the total number of messages in the queue that are either
	// not picked up or is not completely processed by the worker
	// SQS: ApproximateNumberOfMessagesVisible + ApproximateNumberOfMessagesNotVisible
	Messages int32
	// messagesSent is the number of messages sent to the queue per minute
	// SQS: NumberOfMessagesSent metric
	// this will help in calculating the desired replicas.
	// It is most useful for workers which process very fast and
	// always has a messages = 0  in the queue
	MessagesSentPerMinute float64
	// idleWorkers tells the number of workers which are idle
	// and not doing any processing.
	idleWorkers int32
	workers     int32

	// secondsToProcessOneJob tells the time to process
	// one job by one worker process
	SecondsToProcessOneJob float64
}

func NewQueues() *Queues {
	return &Queues{
		addCh:               make(chan map[string]QueueSpec),
		deleteCh:            make(chan string),
		listCh:              make(chan chan map[string]QueueSpec),
		updateMessageCh:     make(chan map[string]int32),
		updateMessageSentCh: make(chan map[string]float64),
		idleWorkerCh:        make(chan map[string]int32),
		item:                make(map[string]QueueSpec),
	}
}

func (q *Queues) updateMessage(key string, count int32) {
	q.updateMessageCh <- map[string]int32{
		key: count,
	}
}

func (q *Queues) updateMessageSent(key string, count float64) {
	q.updateMessageSentCh <- map[string]float64{
		key: count,
	}
}

func (q *Queues) updateIdleWorkers(key string, idleWorkers int32) {
	q.idleWorkerCh <- map[string]int32{
		key: idleWorkers,
	}
}

func (q *Queues) Sync(stopCh <-chan struct{}) {
	for {
		select {
		case queueSpecMap := <-q.addCh:
			for key, value := range queueSpecMap {
				q.item[key] = value
			}
			doneQueueSync()
		case message := <-q.updateMessageCh:
			for key, value := range message {
				if _, ok := q.item[key]; !ok {
					continue
				}
				var spec = q.item[key]
				spec.Messages = value
				q.item[key] = spec
			}
			doneQueueSync()
		case messageSent := <-q.updateMessageSentCh:
			for key, value := range messageSent {
				if _, ok := q.item[key]; !ok {
					continue
				}
				var spec = q.item[key]
				spec.MessagesSentPerMinute = value
				q.item[key] = spec
			}
			doneQueueSync()
		case idleStatus := <-q.idleWorkerCh:
			for key, value := range idleStatus {
				if _, ok := q.item[key]; !ok {
					continue
				}
				var spec = q.item[key]
				spec.idleWorkers = value
				q.item[key] = spec
			}
			doneQueueSync()
		case key := <-q.deleteCh:
			_, ok := q.item[key]
			if ok {
				delete(q.item, key)
			}
			doneQueueSync()
		case listResultCh := <-q.listCh:
			listResultCh <- DeepCopyItem(q.item)
		case <-stopCh:
			klog.V(1).Info("Stopping queue syncer gracefully.")
			return
		}
	}
}

func (q *Queues) Add(namespace string, name string, uri string,
	workers int32, secondsToProcessOneJob float64) error {

	if uri == "" {
		klog.Warningf(
			"Queue is empty(or not synced) ignoring the wpa for uri: %s", uri)
		return nil
	}

	queueName := GetQueueName(uri)
	protocol, host, err := parseQueueURI(uri)
	if err != nil {
		return err
	}

	key := getMultiQueueKey(namespace, name, queueName)
	supported, queueServiceName, err := getQueueServiceName(host, protocol)
	if !supported {
		klog.Warningf(
			"Unsupported: %s, skipping wpa: %s", queueServiceName, name)
		return nil
	}

	messages := int32(UnsyncedQueueMessageCount)
	idleWorkers := int32(UnsyncedIdleWorkers)
	messagesSent := float64(UnsyncedMessagesSentPerMinute)
	spec := q.listQueueByNamespace(namespace, name, queueName)
	if spec.Name != "" {
		messages = spec.Messages
		messagesSent = spec.MessagesSentPerMinute
		idleWorkers = spec.idleWorkers
	}

	queueSpec := QueueSpec{
		Name:                   queueName,
		namespace:              namespace,
		uri:                    uri,
		protocol:               protocol,
		host:                   host,
		queueServiceName:       queueServiceName,
		Messages:               messages,
		MessagesSentPerMinute:  messagesSent,
		workers:                workers,
		idleWorkers:            idleWorkers,
		SecondsToProcessOneJob: secondsToProcessOneJob,
	}

	q.addCh <- map[string]QueueSpec{key: queueSpec}
	return nil
}

func (q *Queues) Delete(namespace string, name string, queueName string) error {
	q.deleteCh <- getMultiQueueKey(namespace, name, queueName)
	return nil
}

func (q *Queues) ListAll() map[string]QueueSpec {
	listResultCh := make(chan map[string]QueueSpec)
	q.listCh <- listResultCh
	return <-listResultCh
}

func (q *Queues) List(queueServiceName string) map[string]QueueSpec {
	filteredQueues := make(map[string]QueueSpec)
	for key, spec := range q.ListAll() {
		if spec.queueServiceName == queueServiceName {
			filteredQueues[key] = spec
		}
	}
	return filteredQueues
}

func (q *Queues) ListQueue(key string) QueueSpec {
	item := q.ListAll()
	if _, ok := item[key]; !ok {
		return QueueSpec{}
	}

	return item[key]
}

// ListMultiQueues fetches all queue specs with the given key prefix and returns
// a map of queue uri to queue spec
func (q *Queues) ListMultiQueues(key string) map[string]QueueSpec {
	items := q.ListAll()
	specs := make(map[string]QueueSpec)
	for k, v := range items {
		if strings.HasPrefix(k, key) {
			specs[v.uri] = v
		}
	}
	return specs
}

// ListActiveMultiQueues fetches all active queue specs with the given key prefix
// and returns a map of queue uri to queue spec
func (q *Queues) ListActiveMultiQueues(key string) (map[string]QueueSpec, error) {
	config := statsig.GetConfig(statsig.User{UserID: key}, PausedQueuesDynamicConfig)
	items := q.ListAll()
	specs := make(map[string]QueueSpec)
	for k, v := range items {
		queueName := GetQueueName(v.uri)
		if !config.GetBool(queueName, false) && strings.HasPrefix(k, key) && v.Messages > 0 {
			specs[v.uri] = v
		}
	}
	if len(specs) == 0 {
		return nil, errors.Errorf("No active queues found for key: %s. Please check the dynamic config in statsig console %s", key, PausedQueuesDynamicConfig)
	}
	return specs, nil
}

func (q *Queues) listQueueByNamespace(namespace string, name string, queueName string) QueueSpec {
	return q.ListQueue(getMultiQueueKey(namespace, name, queueName))
}

func (q *Queues) GetQueueInfo(
	namespace string, name string, qName string) (string, int32, float64, int32) {

	spec := q.listQueueByNamespace(namespace, name, qName)
	if spec.Name == "" {
		return "", 0, 0.0, 0
	}

	return spec.Name, spec.Messages,
		spec.MessagesSentPerMinute, spec.idleWorkers
}

func parseQueueURI(uri string) (string, string, error) {
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return "", "", err
	}

	return parsedURI.Scheme, parsedURI.Host, nil
}

func GetQueueName(name string) string {
	splitted := strings.Split(name, "/")
	return splitted[len(splitted)-1]
}

func getMultiQueueKey(namespace string, name string, queueName string) string {
	return strings.Join([]string{namespace, name, queueName}, "/")
}

func DeepCopyItem(original map[string]QueueSpec) map[string]QueueSpec {
	copy := make(map[string]QueueSpec)
	for key, value := range original {
		copy[key] = value
	}
	return copy
}

func Aggregate(qSpecs map[string]QueueSpec) (int32, float64, int32) {
	totalMessages := int32(0)
	totalMessagesSentPerMinute := float64(0)

	if len(qSpecs) == 0 {
		return totalMessages, totalMessagesSentPerMinute, UnsyncedIdleWorkers
	}

	for _, qSpec := range qSpecs {
		totalMessages += qSpec.Messages
		totalMessagesSentPerMinute += qSpec.MessagesSentPerMinute
	}

	idleWorkers := int32(math.MaxInt32)
	for _, qSpec := range qSpecs {
		if qSpec.idleWorkers == UnsyncedIdleWorkers {
			return totalMessages, totalMessagesSentPerMinute, UnsyncedIdleWorkers
		} else if qSpec.idleWorkers < idleWorkers {
			idleWorkers = qSpec.idleWorkers
		}
	}

	return totalMessages, totalMessagesSentPerMinute, idleWorkers
}
