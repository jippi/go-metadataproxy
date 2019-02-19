package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
)

const (
	awsTimeLayoutResponse = "2006-01-02T15:04:05Z"
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

func readRoleFromAWS(role string, request *Request) (*iam.Role, error) {
	request.log.Infof("Looking for IAM role for %s", role)

	roleObject := &iam.Role{}
	if roleObject, ok := roleCache.Get(role); ok {
		request.setLabel("read_role_from_aws_cache", "hit")
		request.log.Infof("Found IAM role %s in cache", role)
		return roleObject.(*iam.Role), nil
	}

	request.setLabel("read_role_from_aws_cache", "miss")

	if strings.Contains(role, "@") { // IAM_ROLE=my-role@012345678910
		request.log.Infof("Constructing IAM role info for %s manually", role)
		chunks := strings.SplitN(role, "@", 2)
		nameChunks := strings.Split(chunks[0], "/")

		roleObject = &iam.Role{
			Arn:      aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", chunks[1], strings.TrimLeft(chunks[0], "/"))),
			RoleName: aws.String(nameChunks[len(nameChunks)-1]),
		}
	} else if strings.HasPrefix(role, "arn:aws:iam") { // IAM_ROLE=arn:aws:iam::012345678910:role/my-role
		request.log.Infof("Using IAM role ARN as is for %s", role)

		chunks := strings.SplitN(role, ":role/", 2)
		nameChunks := strings.Split(chunks[1], "/")

		roleObject = &iam.Role{
			Arn:      aws.String(role),
			RoleName: aws.String(nameChunks[len(nameChunks)-1]),
		}
	} else { // IAM_ROLE=my-role
		request.log.Infof("Requesting IAM role info for %s from AWS", role)
		req := iamService.GetRoleRequest(&iam.GetRoleInput{
			RoleName: aws.String(role),
		})

		resp, err := req.Send()
		if err != nil {
			return nil, err
		}

		roleObject = resp.Role
	}

	roleCache.Set(role, roleObject, cache.DefaultExpiration)
	return roleObject, nil
}

func assumeRoleFromAWS(arn string, request *Request) (*sts.AssumeRoleOutput, error) {
	request.log.Infof("Looking for STS Assume Role for %s", arn)

	if assumedRole, ok := permissionCache.Get(arn); ok {
		request.setLabel("assume_role_from_aws_cache", "hit")
		request.log.Infof("Found STS Assume Role %s in cache", arn)
		return assumedRole.(*sts.AssumeRoleOutput), nil
	}

	request.setLabel("assume_role_from_aws_cache", "miss")
	request.log.Infof("Requesting STS Assume Role info for %s from AWS", arn)
	req := stsService.AssumeRoleRequest(&sts.AssumeRoleInput{
		RoleArn:         aws.String(arn),
		RoleSessionName: aws.String("go-metadataproxy"),
	})

	assumedRole, err := req.Send()
	if err != nil {
		return nil, err
	}

	ttl := assumedRole.Credentials.Expiration.Sub(time.Now()) - 1*time.Minute
	request.log.Infof("Will cache STS Assumed Role info for %s in %s", arn, ttl.String())
	permissionCache.Set(arn, assumedRole, ttl)
	return assumedRole, nil
}
