FROM ghcr.io/vechain/inspector-app:V1.1.1 as inspector-base

FROM node:22-alpine as builder

WORKDIR /usr/app

COPY --from=inspector-base /usr/share/nginx/html /usr/app/html

# should be built from context of the root of the project
COPY builtin/gen/compiled /usr/app/builtin/gen/compiled
COPY local/builder.js /usr/app/builder.js
COPY local/modify-inspector.js /usr/app/modify-inspector.js

RUN node /usr/app/builder.js
RUN node /usr/app/modify-inspector.js

FROM ghcr.io/vechain/inspector-app:V1.1.1

COPY --from=builder /usr/app/html /usr/share/nginx/html
COPY --from=builder /usr/app/builtin/gen/abis/contracts.json /usr/share/nginx/html/abis/contracts.json
