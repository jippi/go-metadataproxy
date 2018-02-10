package main

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	log "github.com/sirupsen/logrus"
)

var (
	iamService *iam.IAM
	stsService *sts.STS
)

func readRoleFromAWS(role string) (*iam.GetRoleOutput, error) {
	log.Infof("Requesting IAM role info for %s", role)

	req := iamService.GetRoleRequest(&iam.GetRoleInput{
		RoleName: aws.String(role),
	})

	return req.Send()
}

func assumeRoleFromAWS(arn string) (*sts.AssumeRoleOutput, error) {
	log.Infof("Assuming IAM role info for %s", arn)

	req := stsService.AssumeRoleRequest(&sts.AssumeRoleInput{
		RoleArn:         aws.String(arn),
		RoleSessionName: aws.String("go-metadataproxy"),
	})

	return req.Send()
}
