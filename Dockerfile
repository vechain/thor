# Build thor in a stock Go builder container
FROM golang:1.22.4-bookworm as builder

RUN apt-get update && apt-get install -y make gcc libc-dev musl-dev git
WORKDIR  /go/thor
COPY . /go/thor
RUN make all

# Pull thor into a second stage deploy debian container
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && apt-get upgrade -y libssl3 && apt-get clean
COPY --from=builder /go/thor/bin/thor /usr/local/bin/
COPY --from=builder /go/thor/bin/disco /usr/local/bin/
RUN useradd -ms /bin/bash thor
USER thor

EXPOSE 8669 11235 11235/udp 55555/udp
ENTRYPOINT ["thor"]
