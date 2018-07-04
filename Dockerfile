# Build thor in a stock Go builder container
FROM golang:1.10-alpine as builder
RUN apk add --no-cache make gcc musl-dev linux-headers
COPY . /thor RUN cd /thor && make dep && make

# Pull thor into a second stage deploy alpine container
FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /thor/bin/thor /usr/local/bin/
EXPOSE 8669 11235 11235/udp
# set the ENTRYPOINT to thor, just run '-h' at the end to get help.
ENTRYPOINT ["thor"]
