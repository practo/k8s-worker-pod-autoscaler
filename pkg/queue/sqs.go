package queue

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/practo/klog/v2"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/sqs"
)

// SQS is used to by the Poller to get the queue
// information from AWS SQS, it implements the QueuingService interface
type SQS struct {
	name          string
	queues        *Queues
	sqsClientPool map[string]*sqs.SQS
	cwClientPool  map[string]*cloudwatch.CloudWatch

	shortPollInterval time.Duration
	longPollInterval  int64

	// cache the numberOfSentMessages as it is refreshed
	// in aws every 1minute - prevent un-necessary api calls
	cacheSentMessages              *sync.Map
	cacheSentMessagesValidity      time.Duration
	cacheSentMessageslastTimestamp *sync.Map

	// cache the numberOfReceiveMessages as it is refreshed
	// in aws every 1minute - prevent un-necessary api calls
	cacheReceiveMessages              *sync.Map
	cacheReceiveMessagesValidity      time.Duration
	cacheReceiveMessageslastTimestamp *sync.Map
}

func NewSQS(
	name string,
	awsRegions []string,
	queues *Queues,
	shortPollInterval int,
	longPollInterval int) (QueuingService, error) {

	sqsClientPool := make(map[string]*sqs.SQS)
	cwClientPool := make(map[string]*cloudwatch.CloudWatch)

	for _, region := range awsRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region)},
		)

		if err != nil {
			return nil, err
		}

		sqsClientPool[region] = sqs.New(sess)
		cwClientPool[region] = cloudwatch.New(sess)
	}

	return &SQS{
		name:          name,
		queues:        queues,
		sqsClientPool: sqsClientPool,
		cwClientPool:  cwClientPool,

		shortPollInterval: time.Second * time.Duration(shortPollInterval),
		longPollInterval:  int64(longPollInterval),

		cacheSentMessages:              new(sync.Map),
		cacheSentMessagesValidity:      time.Second * time.Duration(60),
		cacheSentMessageslastTimestamp: new(sync.Map),

		cacheReceiveMessages:              new(sync.Map),
		cacheReceiveMessagesValidity:      time.Second * time.Duration(60),
		cacheReceiveMessageslastTimestamp: new(sync.Map),
	}, nil
}

func (s *SQS) getSQSClient(queueURI string) *sqs.SQS {
	return s.sqsClientPool[getRegion(queueURI)]
}

func (s *SQS) getCWClient(queueURI string) (*cloudwatch.CloudWatch, error) {
	client, ok := s.cwClientPool[getRegion(queueURI)]
	if !ok {
		return nil, fmt.Errorf("Client not found for queue: %s\n", queueURI)
	}

	return client, nil
}

func (s *SQS) longPollReceiveMessage(queueURI string) (int32, error) {
	result, err := s.getSQSClient(queueURI).ReceiveMessage(&sqs.ReceiveMessageInput{
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

	sqsClient = s.getSQSClient(queueURI)

	if sqsClient == nil {
		return 0, fmt.Errorf("failed to get SQS client, QUEUE URI: %s", queueURI)
	}

	result, err := sqsClient.GetQueueAttributes(&sqs.GetQueueAttributesInput{
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
	result, err := s.getSQSClient(queueURI).GetQueueAttributes(&sqs.GetQueueAttributesInput{
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

func (s *SQS) getNumberOfMessagesReceived(queueURI string) (float64, error) {
	period := int64(60)
	duration, err := time.ParseDuration("-10m")
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
				MetricName: aws.String("NumberOfMessagesReceived"),
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{
						Name:  aws.String("QueueName"),
						Value: aws.String(path.Base(queueURI)),
					},
				},
			},
			Period: &period,
			Stat:   aws.String("Sum"),
		},
	}

	cwClient, err := s.getCWClient(queueURI)
	if err != nil {
		return 0.0, err
	}

	result, err := cwClient.GetMetricData(&cloudwatch.GetMetricDataInput{
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
		var sum float64
		for i := 0; i < len(result.MetricDataResults[0].Values); i++ {
			sum += *result.MetricDataResults[0].Values[i]
		}
		return sum, nil
	}

	klog.Errorf("NumberOfMessagesReceived Cloudwatch API returned empty result for uri: %q", queueURI)

	return 0.0, nil
}

func (s *SQS) getSentMessageCache(queueURI string) (float64, bool) {
	cache, _ := s.cacheSentMessages.Load(queueURI)
	if cache != nil {
		return cache.(float64), true
	}

	return 0.0, false
}

func (s *SQS) updateSentMessageCache(key string, cache float64) {
	s.cacheSentMessages.Store(key, cache)
}

func (s *SQS) cachedNumberOfSentMessages(queueURI string) (float64, error) {
	lastTimeStamp, _ := s.cacheSentMessageslastTimestamp.Load(queueURI)
	if lastTimeStamp == nil {
		lastTimeStamp = time.Now().UnixNano() -
			(time.Second * time.Duration(70)).Nanoseconds()
	}

	now := time.Now().UnixNano()
	if (lastTimeStamp.(int64) + s.cacheSentMessagesValidity.Nanoseconds()) > now {
		cache, cacheHit := s.getSentMessageCache(queueURI)
		if cacheHit {
			return cache, nil
		}
	}

	messagesSent, err := s.getAverageNumberOfMessagesSent(queueURI)
	if err != nil {
		return messagesSent, err
	}
	s.updateSentMessageCache(queueURI, messagesSent)
	s.cacheSentMessageslastTimestamp.Store(queueURI, now)
	return messagesSent, nil
}

func (s *SQS) getReceiveMessageCache(queueURI string) (float64, bool) {
	cache, _ := s.cacheReceiveMessages.Load(queueURI)
	if cache != nil {
		return cache.(float64), true
	}

	return 0.0, false
}

func (s *SQS) updateReceiveMessageCache(key string, cache float64) {
	s.cacheReceiveMessages.Store(key, cache)
}

func (s *SQS) cachedNumberOfReceiveMessages(queueURI string) (float64, error) {
	lastTimeStamp, _ := s.cacheReceiveMessageslastTimestamp.Load(queueURI)
	if lastTimeStamp == nil {
		lastTimeStamp = time.Now().UnixNano() -
			(time.Second * time.Duration(70)).Nanoseconds()
	}

	now := time.Now().UnixNano()
	if (lastTimeStamp.(int64) + s.cacheReceiveMessagesValidity.Nanoseconds()) > now {
		cache, cacheHit := s.getReceiveMessageCache(queueURI)
		if cacheHit {
			return cache, nil
		}
	}

	messagesReceived, err := s.getNumberOfMessagesReceived(queueURI)
	if err != nil {
		return messagesReceived, err
	}
	s.updateReceiveMessageCache(queueURI, messagesReceived)
	s.cacheReceiveMessageslastTimestamp.Store(queueURI, now)
	return messagesReceived, nil
}

func (s *SQS) getAverageNumberOfMessagesSent(queueURI string) (float64, error) {
	period := int64(60)
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
				MetricName: aws.String("NumberOfMessagesSent"),
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{
						Name:  aws.String("QueueName"),
						Value: aws.String(path.Base(queueURI)),
					},
				},
			},
			Period: &period,
			Stat:   aws.String("Sum"),
		},
	}

	cwClient, err := s.getCWClient(queueURI)
	if err != nil {
		return 0.0, err
	}

	result, err := cwClient.GetMetricData(&cloudwatch.GetMetricDataInput{
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
		var sum float64
		for i := 0; i < len(result.MetricDataResults[0].Values); i++ {
			sum += *result.MetricDataResults[0].Values[i]
		}
		return sum / float64(len(result.MetricDataResults[0].Values)), nil
	}

	klog.Errorf("NumberOfMessagesSent Cloudwatch API returned empty result for uri: %q", queueURI)

	return 0.0, nil
}

func (s *SQS) waitForShortPollInterval() {
	time.Sleep(s.shortPollInterval)
}

// TODO: get rid of string parsing
func getRegion(queueURI string) string {
	regionDns := strings.Split(queueURI, "/")[2]
	return strings.Split(regionDns, ".")[1]
}

func (s *SQS) GetName() string {
	return s.name
}

func (s *SQS) poll(key string, queueSpec QueueSpec) {
	if queueSpec.workers == 0 && queueSpec.messages == 0 && queueSpec.messagesSentPerMinute == 0 {
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
				klog.Errorf("Unable to receive message from queue %q, %v.",
					queueSpec.name, err)
				return
			}
		}

		s.queues.updateMessage(key, messagesReceived)
		return
	}

	if queueSpec.secondsToProcessOneJob != 0.0 {
		messagesSentPerMinute, err := s.cachedNumberOfSentMessages(queueSpec.uri)
		if err != nil {
			klog.Errorf("Unable to fetch no of messages to the queue %q, %v.",
				queueSpec.name, err)
			return
		}
		s.queues.updateMessageSent(key, messagesSentPerMinute)
		klog.V(3).Infof("%s: messagesSentPerMinute=%v", queueSpec.name, messagesSentPerMinute)
	}

	approxMessages, err := s.getApproxMessages(queueSpec.uri)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == sqs.ErrCodeQueueDoesNotExist {
			klog.Errorf("Unable to find queue %q, %v. (checking after 20s)", queueSpec.name, err)
			time.Sleep(20 * time.Second)
			return
		} else if ok && aerr.Code() == "RequestError" {
			klog.Errorf("Unable to perform request get approximate messages %q, %v.",
				queueSpec.name, err)
			return
		} else {
			klog.Errorf("Unable to get approximate messages in queue %q, %v.",
				queueSpec.name, err)
			return
		}
	}
	klog.V(3).Infof("%s: approxMessages=%d", queueSpec.name, approxMessages)

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
			klog.Errorf("Unable to get approximate messages not visible in queue %q, %v.",
				queueSpec.name, err)
			return
		}
	}
	klog.V(3).Infof("approxMessagesNotVisible=%d", approxMessagesNotVisible)

	s.queues.updateMessage(key, approxMessages+approxMessagesNotVisible)

	if approxMessages != 0 {
		s.queues.updateIdleWorkers(key, -1)
		s.waitForShortPollInterval()
		return
	}

	if approxMessagesNotVisible > 0 {
		klog.V(3).Infof("%s: approxMessagesNotVisible > 0, not scaling down", queueSpec.name)
		s.waitForShortPollInterval()
		return
	}

	numberOfMessagesReceived, err := s.cachedNumberOfReceiveMessages(queueSpec.uri)
	if err != nil {
		klog.Errorf("Unable to fetch no of received messages for queue %q, %v.",
			queueSpec.name, err)
		time.Sleep(100 * time.Millisecond)
		return
	}

	var idleWorkers int32
	if numberOfMessagesReceived == 0.0 {
		// this will result in all workers getting scaled down
		idleWorkers = queueSpec.workers
	} else {
		idleWorkers = 0
	}

	klog.V(3).Infof("%s: msgsReceived=%f, workers=%d, idleWorkers=%d",
		queueSpec.name,
		numberOfMessagesReceived,
		queueSpec.workers,
		idleWorkers,
	)
	s.queues.updateIdleWorkers(key, idleWorkers)
	s.waitForShortPollInterval()
	return
}
