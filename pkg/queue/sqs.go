package queue

import (
	"fmt"
	"path"
	"strconv"
	"time"

	"k8s.io/klog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/sqs"
)

// SQS is used to by the Poller to get the queue
// information from AWS SQS, it implements the QueuingService interface
type SQS struct {
	queues    *Queues
	sqsClient *sqs.SQS
	cwClient  *cloudwatch.CloudWatch

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

func NewSQS(
	awsRegion string,
	queues *Queues,
	shortPollInterval int,
	longPollInterval int) (QueuingService, error) {

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)

	if err != nil {
		return nil, err
	}

	return &SQS{
		queues:    queues,
		sqsClient: sqs.New(sess),
		cwClient:  cloudwatch.New(sess),

		shortPollInterval: time.Second * time.Duration(shortPollInterval),
		longPollInterval:  int64(longPollInterval),

		cache:         make(map[string]float64),
		cacheValidity: time.Second * time.Duration(300),
		cacheListCh:   make(chan chan map[string]float64),
		cacheUpdateCh: make(chan map[string]float64),
	}, nil
}

func (s *SQS) longPollReceiveMessage(queueURI string) (int32, error) {
	result, err := s.sqsClient.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl: aws.String(queueURI),
		AttributeNames: aws.StringSlice([]string{
			"SentTimestamp",
		}),
		VisibilityTimeout:   aws.Int64(0),
		MaxNumberOfMessages: aws.Int64(1),
		MessageAttributeNames: aws.StringSlice([]string{
			"All",
		}),
		WaitTimeSeconds: aws.Int64(s.longPollInterval),
	})

	if err != nil {
		return 0, err
	}

	return int32(len(result.Messages)), nil
}

func (s *SQS) getApproxMessages(queueURI string) (int32, error) {
	result, err := s.sqsClient.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueUrl:       &queueURI,
		AttributeNames: []*string{aws.String("ApproximateNumberOfMessages")},
	})

	if err != nil {
		return 0, err
	}

	messages, ok := result.Attributes["ApproximateNumberOfMessages"]
	if !ok {
		return 0, fmt.Errorf("ApproximateNumberOfMessages not found: %+v",
			result.Attributes)
	}

	i64, err := strconv.ParseInt(*messages, 10, 32)
	if err != nil {
		return 0, err
	}

	return int32(i64), nil
}

func (s *SQS) getApproxMessagesNotVisible(queueURI string) (int32, error) {
	result, err := s.sqsClient.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueUrl:       &queueURI,
		AttributeNames: []*string{aws.String("ApproximateNumberOfMessagesNotVisible")},
	})

	if err != nil {
		return 0, err
	}

	messages, ok := result.Attributes["ApproximateNumberOfMessagesNotVisible"]
	if !ok {
		return 0, fmt.Errorf("ApproximateNumberOfMessages not found: %+v",
			result.Attributes)
	}

	i64, err := strconv.ParseInt(*messages, 10, 32)
	if err != nil {
		return 0, err
	}

	return int32(i64), nil
}

func (s *SQS) numberOfEmptyReceives(queueURI string) (float64, error) {
	period := int64(300)
	duration, err := time.ParseDuration("-5m")
	if err != nil {
		return 0.0, err
	}
	endTime := time.Now().Add(duration)
	startTime := endTime.Add(duration)

	query := &cloudwatch.MetricDataQuery{
		Id: aws.String("id1"),
		MetricStat: &cloudwatch.MetricStat{
			Metric: &cloudwatch.Metric{
				Namespace:  aws.String("AWS/SQS"),
				MetricName: aws.String("NumberOfEmptyReceives"),
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{
						Name:  aws.String("QueueName"),
						Value: aws.String(path.Base(queueURI)),
					},
				},
			},
			Period: &period,
			Stat:   aws.String("p99"),
		},
	}

	result, err := s.cwClient.GetMetricData(&cloudwatch.GetMetricDataInput{
		EndTime:           &endTime,
		StartTime:         &startTime,
		MetricDataQueries: []*cloudwatch.MetricDataQuery{query},
	})

	if err != nil {
		return 0.0, err
	}

	if len(result.MetricDataResults) > 1 {
		return 0.0, fmt.Errorf("Expecting cloudwatch metric to return single data point")
	}

	if result.MetricDataResults[0].Values != nil && len(result.MetricDataResults[0].Values) > 0 {
		return *result.MetricDataResults[0].Values[0], nil
	}

	klog.Errorf("Number Of Empty Receives API returned empty result for uri: %q", queueURI)

	return 0.0, nil
}

func (s *SQS) cachedNumberOfEmptyReceives(queueURI string) (float64, error) {
	now := time.Now().UnixNano()
	if (s.lastCachedTimestamp + s.cacheValidity.Nanoseconds()) > now {
		cache, cacheHit := s.getCache(queueURI)
		if cacheHit {
			return cache, nil
		}
	}

	emptyReceives, err := s.numberOfEmptyReceives(queueURI)
	if err != nil {
		return emptyReceives, err
	}
	s.updateCache(queueURI, emptyReceives)
	s.lastCachedTimestamp = now
	return emptyReceives, nil
}

func (s *SQS) listAllCache() map[string]float64 {
	cacheResultCh := make(chan map[string]float64)
	s.cacheListCh <- cacheResultCh
	return <-cacheResultCh
}

func (s *SQS) getCache(queueURI string) (float64, bool) {
	allCache := s.listAllCache()
	if cache, ok := allCache[queueURI]; ok {
		return cache, true
	}
	return 0.0, false
}

func (s *SQS) updateCache(key string, cache float64) {
	s.cacheUpdateCh <- map[string]float64{
		key: cache,
	}
}

func (s *SQS) Sync(stopCh <-chan struct{}) {
	for {
		select {
		case update := <-s.cacheUpdateCh:
			for key, value := range update {
				s.cache[key] = value
			}
		case cacheResultCh := <-s.cacheListCh:
			cacheResultCh <- s.cache
		case <-stopCh:
			klog.Info("Stopping sqs syncer gracefully.")
			return
		}
	}
}

func (s *SQS) waitForShortPollInterval() {
	time.Sleep(s.shortPollInterval)
}

func (s *SQS) poll(key string, queueSpec QueueSpec) {
	if queueSpec.workers == 0 && queueSpec.messages == 0 {
		s.queues.updateIdleWorkers(key, -1)

		// If there are no workers running we do a long poll to find a job(s)
		// in the queue. On finding job(s) we increment the queue message
		// by no of messages received to trigger scale up.
		// Long polling is done to keep SQS api calls to minimum.
		messagesReceived, err := s.longPollReceiveMessage(queueSpec.uri)
		if err != nil {
			aerr, ok := err.(awserr.Error)
			if ok && aerr.Code() == sqs.ErrCodeQueueDoesNotExist {
				klog.Errorf("Unable to find queue %q, %v.", queueSpec.name, err)
				return
			} else if ok && aerr.Code() == "RequestError" {
				klog.Errorf("Unable to perform request long polling %q, %v.",
					queueSpec.name, err)
				return
			} else {
				klog.Fatalf("Unable to receive message from queue %q, %v.",
					queueSpec.name, err)
			}
		}

		s.queues.updateMessage(key, messagesReceived)
		return
	}

	approxMessages, err := s.getApproxMessages(queueSpec.uri)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == sqs.ErrCodeQueueDoesNotExist {
			klog.Errorf("Unable to find queue %q, %v.", queueSpec.name, err)
			return
		} else if ok && aerr.Code() == "RequestError" {
			klog.Errorf("Unable to perform request get approximate messages %q, %v.",
				queueSpec.name, err)
			return
		} else {
			klog.Fatalf("Unable to get approximate messages in queue %q, %v.",
				queueSpec.name, err)
		}
	}
	klog.Infof("%s: approxMessages=%d", queueSpec.name, approxMessages)
	s.queues.updateMessage(key, approxMessages)

	if approxMessages != 0 {
		s.queues.updateIdleWorkers(key, -1)
		s.waitForShortPollInterval()
		return
	}

	// approxMessagesNotVisible is queried to prevent scaling down when their are
	// workers which are doing the processing, so if approxMessagesNotVisible > 0 we
	// do not scale down as those messages are still being processed (and we dont know which worker)
	approxMessagesNotVisible, err := s.getApproxMessagesNotVisible(queueSpec.uri)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == sqs.ErrCodeQueueDoesNotExist {
			klog.Errorf("Unable to find queue %q, %v.", queueSpec.name, err)
			return
		} else if ok && aerr.Code() == "RequestError" {
			klog.Errorf("Unable to perform request get approximate messages not visible %q, %v.",
				queueSpec.name, err)
			return
		} else {
			klog.Fatalf("Unable to get approximate messages not visible in queue %q, %v.",
				queueSpec.name, err)
		}
	}
	// klog.Infof("approxMessagesNotVisible=%d", approxMessagesNotVisible)

	if approxMessagesNotVisible > 0 {
		klog.Infof("%s: approxMessagesNotVisible > 0, not scaling down", queueSpec.name)
		s.waitForShortPollInterval()
		return
	}

	// emptyReceives is queried to find if there are idle workers and scale down to
	// minimum workers, so scale down.
	// TODO: Continuously high throughout workers are impacted by this and does not scale down
	// to a lower value even if it is possible
	emptyReceives, err := s.cachedNumberOfEmptyReceives(queueSpec.uri)
	if err != nil {
		klog.Fatalf("Unable to fetch empty receive metric for queue %q, %v.",
			queueSpec.name, err)
	}

	var idleWorkers int32
	if emptyReceives == 1.0 {
		idleWorkers = queueSpec.workers
	} else {
		idleWorkers = 0
	}

	klog.Infof("%s: emptyReceives=%f, workers=%d, idleWorkers=%d",
		queueSpec.name,
		emptyReceives,
		queueSpec.workers,
		idleWorkers,
	)
	s.queues.updateIdleWorkers(key, idleWorkers)
	s.waitForShortPollInterval()
	return
}
