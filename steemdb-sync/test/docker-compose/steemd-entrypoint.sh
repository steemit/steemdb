#!/bin/bash
# Entrypoint script for steemd container
# Builds command with conditional --stop-replay-at-block parameter

set -e

# Base command
CMD=(
    /usr/local/steemd/bin/steemd
    --replay-blockchain
    --plugin ingest
    --ingest-endpoint "${INGEST_ENDPOINT:-http://cold-ingest:8080/ingest/applied_ops}"
    --ingest-http-timeout "${INGEST_HTTP_TIMEOUT:-5000}"
    --ingest-queue-size "${INGEST_QUEUE_SIZE:-100000}"
    --ingest-batch-size "${INGEST_BATCH_SIZE:-100}"
    --ingest-batch-timeout "${INGEST_BATCH_TIMEOUT:-1000}"
    --data-dir "${DATA_DIR:-/var/steem}"
    --p2p-seed-node ""
)

# Add --stop-replay-at-block if STOP_REPLAY_AT_BLOCK is set and not 0
if [ -n "${STOP_REPLAY_AT_BLOCK}" ] && [ "${STOP_REPLAY_AT_BLOCK}" != "0" ]; then
    CMD+=(--stop-replay-at-block "${STOP_REPLAY_AT_BLOCK}")
fi

# Execute command
exec "${CMD[@]}"
