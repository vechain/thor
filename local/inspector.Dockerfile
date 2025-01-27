FROM ghcr.io/vechain/inspector-app:V1.1.1 as inspector-base

FROM node:22-alpine as builder

WORKDIR /usr/app

COPY --from=inspector-base /usr/share/nginx/html /usr/app/html

# should be built from context of the root of the project
COPY builder.js /usr/app/builder.js
COPY modify-inspector.js /usr/app/modify-inspector.js
COPY inspector-entrypoint.sh /usr/app/entrypoint.sh

RUN npm install --global serve

ENTRYPOINT ["/usr/app/entrypoint.sh"]
