FROM alpine

# we need ca-certificates for any external https communication
RUN apk --update upgrade && \
    apk add curl ca-certificates && \
    update-ca-certificates && \
    rm -rf /var/cache/apk/*

ADD ./build/go-metadataproxy-linux-amd64 /go-metadataproxy
EXPOSE 3000
CMD ["/go-metadataproxy"]
