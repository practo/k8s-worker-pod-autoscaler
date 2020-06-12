package queue

import (
	"net/url"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/beanstalkd/go-beanstalk"
	"k8s.io/klog"
)

// Beanstalk is used to by the Poller to get the queue
// information from Beanstalk, it implements the QueuingService interface
type Beanstalk struct {
	queues   *Queues
	connPool *sync.Map

	shortPollInterval time.Duration
	longPollInterval  int64
}

func NewBeanstalk(
	queues *Queues,
	shortPollInterval int,
	longPollInterval int) (QueuingService, error) {

	return &Beanstalk{
		queues:   queues,
		connPool: new(sync.Map),

		shortPollInterval: time.Second * time.Duration(shortPollInterval),
		longPollInterval:  int64(longPollInterval),
	}, nil
}

func MustParseInt(s string, base int, bitSize int) int32 {
	i, err := strconv.ParseInt(s, base, bitSize)
	if err != nil {
		klog.Fatalf("Error parsing int: %v", err)
	}
	return int32(i)
}

func MustParseUint(s string, base int, bitSize int) uint32 {
	i, err := strconv.ParseUint(s, base, bitSize)
	if err != nil {
		klog.Fatalf("Error parsing int: %v", err)
	}
	return uint32(i)
}

func (b *Beanstalk) getConnection(queueURI string) (*beanstalk.Conn, error) {
	conn, _ := b.connPool.Load(queueURI)
	if conn != nil {
		return conn.(*beanstalk.Conn), nil
	}

	var host, port string
	parsedURI, err := url.Parse(queueURI)
	if err != nil {
		return nil, err
	}
	if host = parsedURI.Hostname(); host == "" {
		host = "localhost"
	}
	if port = parsedURI.Port(); port == "" {
		port = "11300"
	}

	conn, err = beanstalk.Dial("tcp", host+":"+port)
	if err != nil {
		return nil, err
	}

	b.connPool.Store(queueURI, conn)

	return conn.(*beanstalk.Conn), nil
}

func (b *Beanstalk) getTube(queueURI string) (*beanstalk.Tube, error) {
	conn, err := b.getConnection(queueURI)
	if err != nil {
		return nil, err
	}

	return &beanstalk.Tube{Conn: conn, Name: path.Base(queueURI)}, nil
}

func (b *Beanstalk) getStats(queueURI string) (int32, int32, int32, error) {
	tube, err := b.getTube(queueURI)
	if err != nil {
		return 0, 0, 0, err
	}

	output, err := tube.Stats()
	if err != nil {
		return 0, 0, 0, err
	}

	jobsWaiting := MustParseInt(output["current-jobs-ready"], 10, 32)
	idleWorkers := MustParseInt(output["current-waiting"], 10, 32)
	jobsReserved := MustParseInt(output["current-jobs-reserved"], 10, 32)
	return jobsWaiting, idleWorkers, jobsReserved, nil
}

func (b *Beanstalk) longPollReceiveMessage(queueURI string) (int32, int32, error) {
	conn, err := b.getConnection(queueURI)
	if err != nil {
		return 0, 0, err
	}

	tubeSet := beanstalk.NewTubeSet(conn, path.Base(queueURI))
	id, _, err := tubeSet.Reserve(
		time.Duration(b.longPollInterval) * time.Second,
	)
	e, ok := err.(beanstalk.ConnError)
	if ok && e.Err == beanstalk.ErrTimeout {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}

	statsJob, err := conn.StatsJob(id)
	if err != nil {
		return 0, 0, err
	}

	conn.Release(id, MustParseUint(statsJob["pri"], 10, 32), 0)

	return 1, 0, nil
}

// TODO: need to get this data from some source
// like: Prometheus: https://github.com/practo/beanstalkd_exporter
func (b *Beanstalk) getAverageNumberOfMessagesSent(queueURI string) (float64, error) {
	return 0.0, nil
}

func (b *Beanstalk) getApproxMessages(queueURI string) (int32, error) {
	jobsWaiting, _, _, err := b.getStats(queueURI)
	if err != nil {
		return jobsWaiting, err
	}

	return jobsWaiting, nil
}

func (b *Beanstalk) getApproxMessagesNotVisible(queueURI string) (int32, error) {
	_, _, jobsReserved, err := b.getStats(queueURI)
	if err != nil {
		return jobsReserved, err
	}

	return jobsReserved, nil
}

func (b *Beanstalk) getIdleWorkers(queueURI string) (int32, error) {
	_, idleWorkers, _, err := b.getStats(queueURI)
	if err != nil {
		return idleWorkers, err
	}

	return idleWorkers, nil
}

func (b *Beanstalk) Sync(stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			klog.Info("Stopping beanstalk syncer gracefully.")
			return
		}
	}
}

func (b *Beanstalk) waitForShortPollInterval() {
	time.Sleep(b.shortPollInterval)
}

func (b *Beanstalk) poll(key string, queueSpec QueueSpec) {
	if queueSpec.workers == 0 && queueSpec.messages == 0 && queueSpec.messagesSentPerMinute == 0 {
		b.queues.updateIdleWorkers(key, -1)

		// If there are no workers running we do a long poll to find a job(s)
		// in the queue. On finding job(s) we increment the queue message
		// by no of messages received to trigger scale up.
		messagesReceived, idleWorkers, err := b.longPollReceiveMessage(queueSpec.uri)
		e, ok := err.(beanstalk.ConnError)
		if ok && e.Err == beanstalk.ErrNotFound {
			return
		}
		if err != nil {
			klog.Errorf("Unable to perform request long polling %q, %v.",
				queueSpec.name, err)
			return
		}

		b.queues.updateMessage(key, messagesReceived)
		b.queues.updateIdleWorkers(key, idleWorkers)
		return
	}

	// TODO: beanstalk does not support secondsToProcessOneJob at present
	if queueSpec.secondsToProcessOneJob != 0.0 {
		// TODO: this should be uncommented when getAverageNumberOfMessagesSent
		// comes live
		// messagesSentPerMinute, err := b.getAverageNumberOfMessagesSent(queueSpec.uri)
		// if err != nil {
		// 	klog.Errorf("Unable to fetch no of messages to the queue %q, %v.",
		// 		queueSpec.name, err)
		// }
		// b.queues.updateMessageSent(key, messagesSentPerMinute)
		// klog.Infof("%s: messagesSentPerMinute=%v", queueSpec.name, messagesSentPerMinute)
	}

	approxMessages, err := b.getApproxMessages(queueSpec.uri)
	if err != nil {
		klog.Errorf("Unable to get approximate messages in queue %q, %v.",
			queueSpec.name, err)
		return
	}
	klog.Infof("%s: approxMessages=%d", queueSpec.name, approxMessages)
	b.queues.updateMessage(key, approxMessages)

	if approxMessages != 0 {
		b.queues.updateIdleWorkers(key, -1)
		b.waitForShortPollInterval()
		return
	}

	// approxMessagesNotVisible is queried to prevent scaling down when their are
	// workers which are doing the processing, so if approxMessagesNotVisible > 0 we
	// do not scale down as those messages are still being processed (and we dont know which worker)
	approxMessagesNotVisible, err := b.getApproxMessagesNotVisible(queueSpec.uri)
	if err != nil {
		klog.Errorf("Unable to get approximate messages not visible in queue %q, %v.",
			queueSpec.name, err)
		return
	}
	// klog.Infof("approxMessagesNotVisible=%d", approxMessagesNotVisible)

	if approxMessagesNotVisible > 0 {
		klog.Infof("%s: approxMessagesNotVisible > 0, not scaling down", queueSpec.name)
		b.waitForShortPollInterval()
		return
	}

	idleWorkers, err := b.getIdleWorkers(queueSpec.uri)
	if err != nil {
		klog.Errorf("Unable to fetch idle workers %q, %v.",
			queueSpec.name, err)
		time.Sleep(100 * time.Millisecond)
		return
	}

	klog.Infof("%s: workers=%d, idleWorkers=%d",
		queueSpec.name,
		queueSpec.workers,
		idleWorkers,
	)
	b.queues.updateIdleWorkers(key, idleWorkers)
	b.waitForShortPollInterval()
	return
}
