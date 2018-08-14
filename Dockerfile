FROM golang:1.8

WORKDIR /go/src/relay

RUN go get -d -v github.com/bklimt/relay
RUN go install -v github.com/bklimt/relay/...

ENV GOOGLE_APPLICATION_CREDENTIALS /etc/relay/credentials.json
CMD ["relay"]
