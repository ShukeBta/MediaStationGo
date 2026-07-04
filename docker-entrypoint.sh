#!/bin/sh
set -eu

run_uid="${PUID:-$(id -u mediastation 2>/dev/null || echo 1000)}"
run_gid="${PGID:-$(id -g mediastation 2>/dev/null || echo 1000)}"

case "$run_uid" in
  ''|*[!0-9]*)
    echo "PUID must be a numeric uid, got: $run_uid" >&2
    exit 1
    ;;
esac

case "$run_gid" in
  ''|*[!0-9]*)
    echo "PGID must be a numeric gid, got: $run_gid" >&2
    exit 1
    ;;
esac

if [ "$run_uid" = "0" ]; then
  exec mediastation-go
fi

chown -R "$run_uid:$run_gid" /data /cache 2>/dev/null || true
chown "$run_uid:$run_gid" /media 2>/dev/null || true

exec su-exec "$run_uid:$run_gid" mediastation-go
