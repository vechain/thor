# Build thor in a stock Go builder container
FROM golang:alpine as builder

RUN apk add --no-cache make gcc musl-dev linux-headers git


RUN git clone https://github.com/vechain/thor.git
WORKDIR  /go/thor
RUN git checkout $(git describe --tags `git rev-list --tags --max-count=1`)
RUN make all

# Pull thor into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates
COPY --from=builder /go/thor/bin/thor /usr/local/bin/
COPY --from=builder /go/thor/bin/disco /usr/local/bin/

EXPOSE 8669 11235 11235/udp 55555/udp
ENTRYPOINT ["thor"]