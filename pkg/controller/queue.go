package controller

import (
	"net/url"
	"regexp"
	"strings"

	"k8s.io/klog"
)

const (
	QueueProviderSQS                 = "sqs"
	QueueProviderBeanstalk           = "beanstalk"
	NotIntitializedQueueMessageCount = -1
)

// Queues maintains a list of all queues as specified in WPAs in memory
// The list is kept in sync with the wpa objects
type Queues struct {
	addCh           chan map[string]*QueueSpec
	deleteCh        chan string
	updateMessageCh chan map[string]int
	item            map[string]*QueueSpec `json:"queues"`
}

// QueueSpec is the specification for a single queue
type QueueSpec struct {
	namespace string `json:"namespace"`
	name      string `json:"name"`
	protocol  string `json:"protocol"`
	host      string `json:"host"`
	provider  string `json:"provider"`
	messages  int    `json:"messages"`
}

func NewQueues(addCh chan map[string]*QueueSpec, deleteCh chan string, updateMessageCh chan map[string]int) *Queues {
	return &Queues{
		addCh:           addCh,
		deleteCh:        deleteCh,
		updateMessageCh: updateMessageCh,
		item:            make(map[string]*QueueSpec),
	}
}

func (q *Queues) SyncQueues() {
	for {
		select {
		case queueSpecMap := <-q.addCh:
			for key, value := range queueSpecMap {
				if _, ok := q.item[key]; ok {
					continue
				}
				q.item[key] = value
			}
		case message := <-q.updateMessageCh:
			for key, value := range message {
				if _, ok := q.item[key]; !ok {
					continue
				}
				q.item[key].messages = value
			}
		case key := <-q.deleteCh:
			_, ok := q.item[key]
			if ok {
				delete(q.item, key)
			}
		}
	}
}

func (q *Queues) add(namespace string, name string, uri string) error {
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

	queueSpec := &QueueSpec{
		namespace: namespace,
		name:      queueName,
		protocol:  protocol,
		host:      host,
		provider:  provider,
		messages:  NotIntitializedQueueMessageCount,
	}

	q.addCh <- map[string]*QueueSpec{key: queueSpec}
	return nil
}

func (q *Queues) delete(namespace string, name string) error {
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

func getProvider(host string, protocol string) (bool, string, error) {
	matched, err := regexp.MatchString("^sqs.[a-z][a-z]-[a-z]*-[0-9]{1}.amazonaws.com", host)
	if err != nil {
		return false, "", nil
	}

	if matched {
		return true, QueueProviderSQS, nil
	}

	if protocol == "beanstalk" {
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
