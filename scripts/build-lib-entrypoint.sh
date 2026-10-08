#!/bin/bash
set -e

USER_ID=${LOCAL_UID:-1000}
GROUP_ID=${LOCAL_GID:-1000}

if ! getent group appgroup > /dev/null 2>&1; then
    groupadd -g $GROUP_ID appgroup
fi

if ! getent passwd appuser > /dev/null 2>&1; then
    useradd -u $USER_ID -g $GROUP_ID -m -s /bin/bash appuser
fi

mkdir -p /dist
chown -R appuser:appgroup /dist

exec gosu appuser go build -buildmode=c-shared -o dist/libsyncai_worker.so ./cmd/lib
