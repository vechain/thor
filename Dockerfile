# Build thor in a stock Go builder container
FROM golang:1.25.4-alpine3.21 AS builder

RUN apk add --no-cache make gcc musl-dev linux-headers git
WORKDIR  /go/thor
COPY . /go/thor
RUN make all

# Pull thor into a second stage deploy alpine container
FROM alpine:3.21.3

RUN apk add --no-cache ca-certificates
RUN apk update && \
    apk add --no-cache libssl3=3.3.5-r0 libcrypto3=3.3.5-r0 busybox=1.37.0-r14
COPY --from=builder /go/thor/bin/thor /usr/local/bin/
COPY --from=builder /go/thor/bin/disco /usr/local/bin/
RUN adduser -D -s /bin/ash thor
USER thor

EXPOSE 8669 11235 11235/udp 55555/udp
ENTRYPOINT ["thor"]
