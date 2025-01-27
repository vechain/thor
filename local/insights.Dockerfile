FROM ghcr.io/vechain/insight-app:master as insight-base

FROM node:22-alpine as builder

COPY --from=insight-base /usr/share/nginx/html /usr/app/html

WORKDIR /usr/app

COPY modify-insight.js /usr/app/modify-insight.js
COPY insights-entrypoint.sh /usr/app/entrypoint.sh

RUN npm install --global serve

ENTRYPOINT ["/usr/app/entrypoint.sh"]
