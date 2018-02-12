package internal

import (
	"time"

	"github.com/armon/go-metrics"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
)

const (
	awsTimeLayoutResponse = "2006-01-01T15:04:05Z"
)

var (
	iamService      *iam.IAM
	stsService      *sts.STS
	roleCache       = cache.New(1*time.Hour, 15*time.Minute)
	permissionCache = cache.New(5*time.Minute, 10*time.Minute)
)

// ConfigureAWS will setup the iam and sts services needed during normal operations
func ConfigureAWS() {
	log.Info("Creating AWS client")
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		log.Fatalf("Unable to load AWS SDK config, " + err.Error())
	}

	iamService = iam.New(cfg)
	stsService = sts.New(cfg)
}

func readRoleFromAWS(role string, labels []metrics.Label) (*iam.Role, []metrics.Label, error) {
	log.Infof("Looking for IAM role for %s", role)

	if roleObject, ok := roleCache.Get(role); ok {
		labels = append(labels, metrics.Label{Name: "read_role_from_aws_cache", Value: "hit"})

		log.Infof("Found IAM role %s in cache", role)
		return roleObject.(*iam.Role), labels, nil
	}

	labels = append(labels, metrics.Label{Name: "read_role_from_aws_cache", Value: "miss"})

	log.Infof("Requesting IAM role info for %s from AWS", role)
	req := iamService.GetRoleRequest(&iam.GetRoleInput{
		RoleName: aws.String(role),
	})

	resp, err := req.Send()
	if err != nil {
		return nil, labels, err
	}

	roleCache.Set(role, resp.Role, 6*time.Hour)

	return resp.Role, labels, nil
}

func assumeRoleFromAWS(arn string, labels []metrics.Label) (*sts.AssumeRoleOutput, []metrics.Label, error) {
	log.Infof("Looking for STS Assume Role for %s", arn)

	if assumedRole, ok := permissionCache.Get(arn); ok {
		labels = append(labels, metrics.Label{Name: "assume_role_from_aws_cache", Value: "hit"})

		log.Infof("Found STS Assume Role %s in cache", arn)
		return assumedRole.(*sts.AssumeRoleOutput), labels, nil
	}
	labels = append(labels, metrics.Label{Name: "assume_role_from_aws_cache", Value: "miss"})

	log.Infof("Requesting STS Assume Role info for %s from AWS", arn)
	req := stsService.AssumeRoleRequest(&sts.AssumeRoleInput{
		RoleArn:         aws.String(arn),
		RoleSessionName: aws.String("go-metadataproxy"),
	})

	assumedRole, err := req.Send()
	if err != nil {
		return nil, labels, err
	}

	ttl := assumedRole.Credentials.Expiration.Sub(time.Now()) - 1*time.Minute

	log.Infof("Will cache STS Assumed Role info for %s in %s", arn, ttl.String())

	permissionCache.Set(arn, assumedRole, ttl)

	return assumedRole, labels, nil
}
