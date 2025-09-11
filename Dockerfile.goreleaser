# Pull thor into a second stage deploy alpine container
FROM alpine:3.21.3

RUN apk add --no-cache ca-certificates
RUN apk upgrade libssl3 libcrypto3
COPY thor /usr/local/bin
COPY disco /usr/local/bin
RUN adduser -D -s /bin/ash thor
USER thor

EXPOSE 8669 11235 11235/udp 55555/udp
ENTRYPOINT ["thor"]
