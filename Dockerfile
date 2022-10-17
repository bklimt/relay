FROM golang:1.8

WORKDIR /go/src/relay

RUN go get -d -v github.com/bklimt/relay
RUN go install -v github.com/bklimt/relay/...

ENV KLIMT_RELAY_CONFIG /etc/relay/config.json
ENV GOOGLE_APPLICATION_CREDENTIALS /etc/relay/credentials.json
ENV KLIMT_RELAY_CONFIG /etc/relay/config.json
CMD ["relay"]
