FROM ghcr.io/vechain/insight-app:master as insight-base

FROM node:22-alpine as builder

COPY --from=insight-base /usr/share/nginx/html /usr/app/html

WORKDIR /usr/app

COPY local/modify-insight.js /usr/app/modify-insight.js

RUN node /usr/app/modify-insight.js

FROM ghcr.io/vechain/insight-app:master

COPY --from=builder /usr/app/html /usr/share/nginx/html
