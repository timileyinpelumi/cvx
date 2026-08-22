#!/bin/sh
# Start both halves and tie their lifetimes together: if either dies the
# container dies, so the platform restarts a whole, working instance rather
# than leaving a half-serving one up.
#
# Deliberately a poll rather than "wait -n": BusyBox ash does not support it,
# and the silent result is a container that stays up with a dead API behind a
# live web tier, which is the exact failure this script exists to prevent.
set -u

/app/cvx &
api=$!

node /app/web/server.js &
web=$!

stop() {
  kill -TERM "$api" "$web" 2>/dev/null || true
  wait "$api" 2>/dev/null || true
  wait "$web" 2>/dev/null || true
}
trap 'stop; exit 143' TERM INT

while kill -0 "$api" 2>/dev/null && kill -0 "$web" 2>/dev/null; do
  sleep 2
done

if ! kill -0 "$api" 2>/dev/null; then
  echo "entrypoint: the api process exited; stopping the container" >&2
else
  echo "entrypoint: the web process exited; stopping the container" >&2
fi

stop
exit 1
