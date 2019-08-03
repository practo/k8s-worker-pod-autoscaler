package queue

import (
	"k8s.io/klog"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type SQSPoller struct {
	client *sqs.SQS
	queues *Queues
}

func NewSQSPoller(awsRegion string, queues *Queues) (Poller, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return nil, err
	}
	return &SQSPoller{
		client: sqs.New(sess),
		queues: queues,
	}, nil
}

func (s *SQSPoller) GetJobs(queueName string) int32 {
	return 0
}

func (s *SQSPoller) GetEmptyReceives(queueName string) int32 {
	return 0
}

func (s *SQSPoller) Run() {
	for {
		queues := s.queues.List()
		for key, value := range queues {
			klog.Infof("ListerResult %s %s", key, value.name)
		}
		time.Sleep(time.Second * 3)
	}
}
