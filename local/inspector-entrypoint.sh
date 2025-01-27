#!/bin/sh

node /usr/app/builder.js
node /usr/app/modify-inspector.js

ROOT_DIR=/usr/app/html

for file in $ROOT_DIR/js/app.*.js*;
do
  echo "Processing $file ...";
  sed -i 's|'VUE_APP_SOLO_URL_PLACEHOLDER'|'${VUE_APP_SOLO_URL}'|g' $file
done

serve -l tcp://0.0.0.0:80 /usr/app/html
