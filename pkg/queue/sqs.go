package queue

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type SQSPoller struct {
	client *sqs.SQS
}

func NewSQSPoller(awsRegion string) (Poller, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return nil, err
	}
	return &SQSPoller{
		client: sqs.New(sess),
	}, nil
}

func (s *SQSPoller) GetJobs(queueName string) int32 {
	return 0
}

func (s *SQSPoller) GetEmptyReceives(queueName string) int32 {
	return 0
}

func (s *SQSPoller) Run() {

}
