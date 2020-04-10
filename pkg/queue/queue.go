package queue

import (
	"net/url"
	"regexp"
	"strings"

	"k8s.io/klog"
)

const (
	QueueProviderSQS              = "sqs"
	QueueProviderBeanstalk        = "beanstalk"
	BenanstalkProtocol            = "beanstalk"
	UnsyncedQueueMessageCount     = -1
	UnsyncedMessagesSentPerMinute = -1
	UnsyncedIdleWorkers           = -1
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
	name      string `json:"name"`
	namespace string `json:"namespace"`
	uri       string `json:"uri"`
	host      string `json:"host"`
	protocol  string `json:"protocol"`
	provider  string `json:"provider"`
	// messages is the number of messages in the queue which have not
	// been picked up for processing by the worker
	// SQS: ApproximateNumberOfMessagesVisible metric
	messages int32 `json:"messages"`
	// messagesSent is the number of messages sent to the queue per minute
	// SQS: NumberOfMessagesSent metric
	// this will help in calculating the desired replicas.
	// It is most useful for workers which process very fast and
	// always has a messages = 0  in the queue
	messagesSentPerMinute float64 `json:"messagesSentPerMinute"`
	// idleWorkers tells the number of workers which are idle
	// and not doing any processing.
	idleWorkers int32 `json:"idleWorkers"`
	workers     int32 `json:"workers"`

	// secondsToProcessOneJob tells the time to process one job by one worker process
	secondsToProcessOneJob float64 `json:"secondsToProcessOneJob"`
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
		case message := <-q.updateMessageCh:
			for key, value := range message {
				if _, ok := q.item[key]; !ok {
					continue
				}
				var spec = q.item[key]
				spec.messages = value
				q.item[key] = spec
			}
		case messageSent := <-q.updateMessageSentCh:
			for key, value := range messageSent {
				if _, ok := q.item[key]; !ok {
					continue
				}
				var spec = q.item[key]
				spec.messagesSentPerMinute = value
				q.item[key] = spec
			}
		case idleStatus := <-q.idleWorkerCh:
			for key, value := range idleStatus {
				if _, ok := q.item[key]; !ok {
					continue
				}
				var spec = q.item[key]
				spec.idleWorkers = value
				q.item[key] = spec
			}
		case key := <-q.deleteCh:
			_, ok := q.item[key]
			if ok {
				delete(q.item, key)
			}
		case listResultCh := <-q.listCh:
			listResultCh <- DeepCopyItem(q.item)
		case <-stopCh:
			klog.Info("Stopping queue syncer gracefully.")
			return
		}
	}
}

func (q *Queues) Add(namespace string, name string, uri string, workers int32, secondsToProcessOneJob float64) error {
	if uri == "" {
		klog.Warningf("Queue is empty(or not synced) ignoring the wpa for uri: %s", uri)
		return nil
	}

	key := getKey(namespace, name)
	queueName := getQueueName(uri)
	protocol, host, err := parseQueueURI(uri)
	if err != nil {
		return err
	}

	found, provider, err := getProvider(host, protocol)
	if !found {
		klog.Warningf("Unsupported queue provider: %s, ignoring wpa: %s", provider, name)
		return nil
	}

	messages := int32(UnsyncedQueueMessageCount)
	idleWorkers := int32(UnsyncedIdleWorkers)
	messagesSent := float64(UnsyncedMessagesSentPerMinute)
	spec := q.listQueueByNamespace(namespace, name)
	if spec.name != "" {
		messages = spec.messages
		idleWorkers = spec.idleWorkers
	}

	queueSpec := QueueSpec{
		name:                   queueName,
		namespace:              namespace,
		uri:                    uri,
		protocol:               protocol,
		host:                   host,
		provider:               provider,
		messages:               messages,
		messagesSentPerMinute:  messagesSent,
		workers:                workers,
		idleWorkers:            idleWorkers,
		secondsToProcessOneJob: secondsToProcessOneJob,
	}

	q.addCh <- map[string]QueueSpec{key: queueSpec}
	return nil
}

func (q *Queues) Delete(namespace string, name string) error {
	q.deleteCh <- getKey(namespace, name)
	return nil
}

func (q *Queues) List() map[string]QueueSpec {
	listResultCh := make(chan map[string]QueueSpec)
	q.listCh <- listResultCh
	return <-listResultCh
}

func (q *Queues) ListQueue(key string) QueueSpec {
	item := q.List()
	if _, ok := item[key]; !ok {
		return QueueSpec{}
	}

	return item[key]
}

func (q *Queues) listQueueByNamespace(namespace string, name string) QueueSpec {
	return q.ListQueue(getKey(namespace, name))
}

func (q *Queues) GetQueueInfo(namespace string, name string) (string, int32, float64, int32) {
	spec := q.listQueueByNamespace(namespace, name)
	if spec.name == "" {
		return "", 0, 0, 0
	}

	return spec.name, spec.messages, spec.messagesSentPerMinute, spec.idleWorkers
}

func parseQueueURI(uri string) (string, string, error) {
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return "", "", err
	}

	return parsedURI.Scheme, parsedURI.Host, nil
}

// getProvider returns the provider name
// TODO: add validation for the queue provider in the wpa custom resource
func getProvider(host string, protocol string) (bool, string, error) {
	matched, err := regexp.MatchString("^sqs.[a-z][a-z]-[a-z]*-[0-9]{1}.amazonaws.com", host)
	if err != nil {
		return false, "", nil
	}

	if matched {
		return true, QueueProviderSQS, nil
	}

	if protocol == BenanstalkProtocol {
		return true, QueueProviderBeanstalk, nil
	}

	return false, "", nil
}

func getQueueName(name string) string {
	splitted := strings.Split(name, "/")
	return splitted[len(splitted)-1]
}

func getKey(namespace string, name string) string {
	return namespace + "/" + name
}

func DeepCopyItem(original map[string]QueueSpec) map[string]QueueSpec {
	copy := make(map[string]QueueSpec)
	for key, value := range original {
		copy[key] = value
	}
	return copy
}
