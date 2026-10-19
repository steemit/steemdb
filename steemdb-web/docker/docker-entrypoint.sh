#!/bin/sh
# Container entrypoint: hands off to supervisord (nginx + steemdb-web).
#
# Web analytics is deliberately NOT injected into the static HTML here. The
# previous mechanism sed-replaced a __GA_ID__ placeholder in index.html on
# every container start (with a pristine template copy to stay idempotent);
# the web service now serves the operator's selection (ANALYTICS_PROVIDER /
# GA_ID / UMAMI_URL) via GET /api/v1/meta, and the SPA loads the matching
# third-party script on the client. See:
#   steemdb-web/internal/api/meta_handler.go   (what /api/v1/meta serves)
#   steemdb-frontend/src/lib/analytics.ts      (client-side loader)
# The image therefore ships tracking-free; nothing is baked in.
set -e

exec /usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf
