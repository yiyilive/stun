FROM golang:1.13

COPY . /go/src/github.com/yiyilive/stun

RUN go test github.com/yiyilive/stun
