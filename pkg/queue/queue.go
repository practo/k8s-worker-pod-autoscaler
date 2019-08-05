package queue

import (
	"net/url"
	"regexp"
	"strings"

	"k8s.io/klog"
)

const (
	QueueProviderSQS          = "sqs"
	QueueProviderBeanstalk    = "beanstalk"
	BenanstalkProtocol        = "beanstalk"
	UnsyncedQueueMessageCount = -1
)

// Queues maintains a list of all queues as specified in WPAs in memory
// The list is kept in sync with the wpa objects
type Queues struct {
	addCh           chan map[string]*QueueSpec
	deleteCh        chan string
	listCh          chan map[string]*QueueSpec
	updateMessageCh chan map[string]int32
	idleWorkerCh    chan map[string]int32
	item            map[string]*QueueSpec `json:"queues"`
}

// QueueSpec is the specification for a single queue
type QueueSpec struct {
	name        string `json:"name"`
	namespace   string `json:"namespace"`
	uri         string `json:"uri"`
	host        string `json:"host"`
	protocol    string `json:"protocol"`
	provider    string `json:"provider"`
	messages    int32  `json:"messages"`
	idleWorkers int32  `json:"idleWorkers"`
	workers     int32  `json:"workers"`
}

func NewQueues() *Queues {
	return &Queues{
		addCh:           make(chan map[string]*QueueSpec),
		deleteCh:        make(chan string),
		listCh:          make(chan map[string]*QueueSpec),
		updateMessageCh: make(chan map[string]int32),
		idleWorkerCh:    make(chan map[string]int32),
		item:            make(map[string]*QueueSpec),
	}
}

func (q *Queues) List() map[string]*QueueSpec {
	return <-q.listCh
}

func (q *Queues) listQueueSpec(namespace string, name string) *QueueSpec {
	key := getKey(namespace, name)
	item := q.List()
	if _, ok := item[key]; !ok {
		return nil
	}
	return item[key]
}

func (q *Queues) GetQueueInfo(namespace string, name string) (string, int32, int32) {
	spec := q.listQueueSpec(namespace, name)
	if spec == nil {
		return "", 0, 0
	}

	return spec.name, spec.messages, spec.idleWorkers
}

func (q *Queues) ListSync() {
	for {
		q.listCh <- q.item
	}
}

func (q *Queues) updateMessage(key string, count int32) {
	q.updateMessageCh <- map[string]int32{
		key: count,
	}
	// We assume if the queue length is not zero
	// then there are no idle workers
	if count != 0 {
		q.updateIdleWorkers(key, int32(0))
	}
}

func (q *Queues) updateIdleWorkers(key string, idleWorkers int32) {
	q.idleWorkerCh <- map[string]int32{
		key: idleWorkers,
	}
}

func (q *Queues) Sync() {
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
				q.item[key].messages = value
			}
		case idleStatus := <-q.idleWorkerCh:
			for key, value := range idleStatus {
				if _, ok := q.item[key]; !ok {
					continue
				}
				q.item[key].idleWorkers = value
			}
		case key := <-q.deleteCh:
			_, ok := q.item[key]
			if ok {
				delete(q.item, key)
			}
		}
	}
}

func (q *Queues) Add(namespace string, name string, uri string, workers int32) error {

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
	spec := q.listQueueSpec(namespace, name)
	if spec != nil {
		messages = spec.messages
	}

	queueSpec := &QueueSpec{
		name:        queueName,
		namespace:   namespace,
		uri:         uri,
		protocol:    protocol,
		host:        host,
		provider:    provider,
		messages:    messages,
		workers:     workers,
		idleWorkers: 0,
	}

	q.addCh <- map[string]*QueueSpec{key: queueSpec}
	return nil
}

func (q *Queues) Delete(namespace string, name string) error {
	q.deleteCh <- getKey(namespace, name)
	return nil
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
