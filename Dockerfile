# Setup Builder Image
FROM golang:1.10-alpine as builder
RUN apk add --no-cache make gcc musl-dev linux-headers git

# Setup Source
WORKDIR /go/src/github.com/vechain/thor
RUN git clone https://github.com/vechain/thor.git .
RUN git submodule update --init

# Compile
RUN make all

# Switch image to a smaller one
FROM alpine
COPY --from=builder /go/src/github.com/vechain/thor/bin /usr/local/bin

# Declare execution command
ENTRYPOINT ["thor"]
EXPOSE 8669 11235 11235/udp
