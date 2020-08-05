# go-metadataproxy

The go-metadataproxy is used to allow containers to acquire IAM roles. By metadata we mean [EC2 instance meta data](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html) which is normally available to EC2 instances. This proxy exposes the meta data to containers inside or outside of EC2 hosts, allowing you to provide scoped IAM roles to individual containers, rather than giving them the full IAM permissions of an IAM role or IAM user.

## Installation

From inside of the repo run the following commands:

```bash
dep ensure
go install
```

## Configuration

The go-metadataproxy has 1 mode of operation, running in AWS where it simply proxies most routes to the real metadata service.

### AWS credentials

go-metadataproxy relies on AWS Go SDK for its AWS credentials. If metadata
IAM credentials are available, it will use this. Otherwise, you'll need to use
.aws/credentials or environment variables to specify the IAM
credentials before the service is started.

### Role assumption

For IAM routes, the go-metadataproxy will use STS to assume roles for containers.
To do so it takes the incoming IP address of metadata requests and finds the
running docker container associated with the IP address. It uses the value of
the container's `IAM_ROLE` environment variable as the role it will assume. It
then assumes the role and gives back STS credentials in the metadata response.

STS-attained credentials are cached and automatically rotated as they expire.

#### Container-specific roles

To specify the role of a container, simply launch it with the `IAM_ROLE`
environment variable set to the IAM role you wish the container to run with.

```shell
docker run -e IAM_ROLE=my-role ubuntu:14.04
```

#### Configurable Behavior

There are a number of environment variables that can be set to tune
metadata proxy's behavior. They can either be exported by the start
script, or set via docker environment variables.

| Variable | Type | Default | Description |
| -------- | ---- | ------- | ----------- |
| `DEFAULT_ROLE` | String | | Role to use if IAM\_ROLE is not set in a container's environment. If unset the container will get no IAM credentials. |
| `DEFAULT_ACCOUNT_ID` | String | | The default account ID to assume roles in, if IAM\_ROLE does not contain account information. If unset, go-metadataproxy will attempt to lookup role ARNs using iam:GetRole. |
| `LOG_LEVEL` | String | "info" | Change the log level (`debug`, `info`, `warning`, `error`, `fatal`, `panic`) |
| `LOG_FORMAT` | String | "text" | Change the log format (`text`, `json`, `gelf`) |
| `DOCKER_URL` | String | unix://var/run/docker.sock | Url of the docker daemon. The default is to access docker via its socket. |
| `ROLE_CACHE_OFFSET` | String | | (Optional) Time to substract from Role cache (default: `15m`) (Example: `5m`, `60s`, `5m30s`) |
| `NEWRELIC_APP_NAME` | String | | (Optional) NewRelic application name. |
| `NEWRELIC_LICENSE` | String | | (Optional) NewRelic license key. |
| `COPY_DOCKER_LABELS` | String | | (Optional) a comma separated list of optional case-senstivie Docker labels to copy into telemetry labels. When copied to telemetry label, the string is automatically lower-cased. (example `COPY_DOCKER_LABELS=PROJECT_VERSION,SOMETHING_ELSE`) |
| `COPY_DOCKER_ENV` | String | | (Optional) a comma separated list of optional case-senstivie Docker env key/value to copy into telemetry labels. When copied to telemetry label, the string is automatically lower-cased. (example `COPY_DOCKER_ENV=PROJECT_VERSION,SOMETHING_ELSE`) |
| `STATSITE_ADDR` | String | | (Optional) Address for a `statsite` server. |
| `STATSD_ADDR` | String | | (Optional) Address for a `statsd` server. |
| `DATADOG_ADDR` | String | | (Optional) Address for a `DataDog statsd` server. |
| `ENABLE_PROMETHEUS` | Bool | | (Optional) Enable `Prometheus` endpoint. Exposed at `/metrics` endpoint |

#### Telemetry

Labels will be emitted as tags for backends using that.

- `api_version` will be set if for all requests expect `/` (which don't contain the meta-data version in the url path)
- `handler_name` will be set to the internal method being used to serve the request
  - `iam-info-handler` will be used for `/{api_version}/meta-data/iam/info`
  - `iam-security-credentials-name` will be used for `/{api_version}/meta-data/iam/security-credentials/`
  - `iam-security-crentials-for-role` will be used for `/{api_version}/meta-data/iam/security-credentials/{requested_role}`
  - `metrics` will be used for `/metrics`
  - `passthrough` will be used for all other requests
- `role_name` will be included if go-metadataproxy found a IAM role during the request
- `request_path` is the full URL path for the request
- `remote_addr` is the remote address requesting the metadata api (typically the container IP)
- `response_code` is the response code to the client connecting to go-metadataproxy. All failures result in a `404` code, otherwise `200`
  - `error_description` If the `response_code` is `404`, this label will contain a description of why - otherwise omitted
- `service` Always set to `go-metadataproxy`

Additional labels from `COPY_DOCKER_LABELS` and `COPY_DOCKER_ENV` will be appended to the list above.

| Key | Type | Labels | Description |
| --- | ---- | ------ | ----------- |
| `metadataproxy.http_request` | `counter` | `api_version`, `request_path`, `response_code`, `error_description`, `role_name`, `handler_name`, `service` | Emitted for each HTTP request proxied, availbility of the labels depend on the request and AWS response |
| `metadataproxy.aws_response_time` | `gauage` | `api_version`, `request_path`, `response_code`, `role_name`, `handler_name`, `service` | The full request time (in nanoseconds) when talking to AWS meta-data endpoint. |
| `metadataproxy.aws_request_time` | `gauge` | `api_version`, `request_path`, `response_code`, `role_name`, `handler_name`, `service` | The request time (in nanoseconds) when talking to AWS meta-data endpoint. |
| `metadataproxy.aws_connection_time` | `gauge` | `api_version`, `request_path`, `response_code`, `role_name`, `handler_name`, `service` | The connect time (in nanoseconds) when talking to AWS meta-data endpoint. |

#### Default Roles

When no role is matched, `go-metadataproxy` will use the role specified in the
`DEFAULT\_ROLE` `go-metadataproxy` environment variable. If no DEFAULT\_ROLE is
specified as a fallback, then your docker container without an `IAM\_ROLE`
environment variable will fail to retrieve credentials.

#### Role Formats

The following are all supported formats for specifying roles:

- By Role:

    ```shell
    IAM_ROLE=my-role
    ```

- By Role@AccountId

    ```shell
    IAM_ROLE=my-role@012345678910
    ```

- By ARN:

    ```shell
    IAM_ROLE=arn:aws:iam::012345678910:role/my-role
    ```

### Role structure

A useful way to deploy this go-metadataproxy is with a two-tier role
structure:

1. The first tier is the EC2 service role for the instances running
   your containers.  Call it `DockerHostRole`.  Your instances must
   be launched with a policy that assigns this role.

2. The second tier is the role that each container will use.  These
   roles must trust your own account ("Role for Cross-Account
   Access" in AWS terms).  Call it `ContainerRole1`.

3. go-metadataproxy needs to query and assume the container role.  So
   the `DockerHostRole` policy must permit this for each container
   role.  For example:

   ```json
    "Statement": [ {
        "Effect": "Allow",
        "Action": [
            "iam:GetRole",
            "sts:AssumeRole"
        ],
        "Resource": [
            "arn:aws:iam::012345678901:role/ContainerRole1",
            "arn:aws:iam::012345678901:role/ContainerRole2"
        ]
    } ]
    ```

4. Now customize `ContainerRole1` & friends as you like

Note: The `ContainerRole1` role should have a trust relationship that allows it to be assumed by the `user` which is associated to the host machine running the `sts:AssumeRole` command.  An example trust relationship for `ContainRole1` may look like:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::012345678901:root",
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
```

### Routing container traffic to go-metadataproxy

Using iptables, we can forward traffic meant to 169.254.169.254 from docker0 to
the go-metadataproxy. The following example assumes the go-metadataproxy is run on
the host, and not in a container:

```bash
/sbin/iptables \
  --append PREROUTING \
  --destination 169.254.169.254 \
  --protocol tcp \
  --dport 80 \
  --in-interface docker0 \
  --jump DNAT \
  --table nat \
  --to-destination 127.0.0.1:8000 \
  --wait
```

If you'd like to start the go-metadataproxy in a container, it's recommended to
use host-only networking. Also, it's necessary to volume mount in the docker
socket, as go-metadataproxy must be able to interact with docker.

Be aware that non-host-mode containers will not be able to contact
127.0.0.1 in the host network stack.  As an alternative, you can use
the meta-data service to find the local address.  In this case, you
probably want to restrict proxy access to the docker0 interface!

```bash
LOCAL_IPV4=$(curl http://169.254.169.254/latest/meta-data/local-ipv4)

/sbin/iptables \
  --append PREROUTING \
  --destination 169.254.169.254 \
  --protocol tcp \
  --dport 80 \
  --in-interface docker0 \
  --jump DNAT \
  --table nat \
  --to-destination $LOCAL_IPV4:8000 \
  --wait

/sbin/iptables \
  --wait \
  --insert INPUT 1 \
  --protocol tcp \
  --dport 80 \
  \! \
  --in-interface docker0 \
  --jump DROP
```

If you run Docker containers within their own bridge network, the network interface will be in format `br-<network-id>` rather than `docker0`.

For example, if a Docker network is created:

```bash
docker network create some-network
d180d436e9c4c4322156140ba04233a530a30966ddbcd7f9be4331724d78f459
```

You may have network interface `br-d180d436e9c4`.

You can setup `iptables` to forward traffic from any such bridge network with a wildcard `+`:

```bash
LOCAL_IPV4=$(curl http://169.254.169.254/latest/meta-data/local-ipv4)

/sbin/iptables \
  --append PREROUTING \
  --destination 169.254.169.254 \
  --protocol tcp \
  --dport 80 \
  --in-interface br-+ \
  --jump DNAT \
  --table nat \
  --to-destination $LOCAL_IPV4:8000 \
  --wait
```

## Run go-metadataproxy without docker

In the following we assume \_my\_config\_ is a bash file with exports for all of
the necessary settings discussed in the configuration section.

```bash
source my_config
cd /srv/go-metadataproxy
go run main.go
```

## Run go-metadataproxy with docker

For production purposes, you'll want to kick up a container to run.
You can build one with the included Dockerfile.  To run, do something like:

```bash
docker run --net=host \
    -v /var/run/docker.sock:/var/run/docker.sock \
    jippi/go-metadataproxy
```

## Attribution

This project is a ~1:1 port of [lyft/metadataproxy](https://github.com/lyft/metadataproxy), done in Go.

## Contributing

### File issues in Github

In general all enhancements or bugs should be tracked via github issues before
PRs are submitted. We don't require them, but it'll help us plan and track.

When submitting bugs through issues, please try to be as descriptive as
possible. It'll make it easier and quicker for everyone if the developers can
easily reproduce your bug.

### Submit pull requests

Our only method of accepting code changes is through github pull requests.
