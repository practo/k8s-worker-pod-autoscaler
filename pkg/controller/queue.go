package controller

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"k8s.io/klog"
)

const (
	QueueProviderSQS       = "sqs"
	QueueProviderBeanstalk = "beanstalk"
)

type Queues struct {
	mutex sync.Mutex

	item map[string]QueueSpec `json:"queues"`
}

type QueueSpec struct {
	namespace string `json:"namespace"`
	name      string `json:"name"`
	protocol  string `json:"protocol"`
	host      string `json:"host"`
	provider  string `json:"provider"`
	messages  int32  `json:"messages"`
}

func NewQueues() *Queues {
	return &Queues{
		item: make(map[string]QueueSpec),
	}
}

func (q *Queues) list() {
	for _, spec := range q.item {
		klog.Infof("%s/%s", spec.namespace, spec.name)
	}
}

// add keeps the in memory Queues updated
// with the objects present in WPA
func (q *Queues) add(namespace string, uri string) error {
	if uri == "" {
		klog.Warningf("Queue is empty(or not synced) ignoring the wpa for namespace: %s", namespace)
		return nil
	}

	name := getQueueName(uri)
	key := getKey(namespace, name)

	if _, ok := q.item[key]; ok {
		return nil
	}

	protocol, host, err := parseQueueURI(uri)
	if err != nil {
		return err
	}

	found, provider, err := getProvider(host, protocol)
	if !found {
		klog.Warningf("Unsupported queue provider: %s, ignoring queue: %s", provider, name)
		return nil
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.item[key] = QueueSpec{
		namespace: namespace,
		name:      name,
		protocol:  protocol,
		host:      host,
		provider:  provider,
		messages:  -1, // -1 denotes it has not been synced yet
	}

	return nil
}

func (q *Queues) getQueueSpec(namespace string, name string) (QueueSpec, error) {
	key := getKey(namespace, name)
	if _, ok := q.item[key]; !ok {
		return QueueSpec{}, fmt.Errorf("Queue: %s not found in namespace %s", name, namespace)
	}
	return q.item[key], nil
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
