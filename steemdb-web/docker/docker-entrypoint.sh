#!/bin/sh
# Container entrypoint: injects the Google Analytics measurement ID into the
# built SPA, then hands off to supervisord.
#
# index.html ships with a __GA_ID__ placeholder. The placeholder itself is a
# no-op (the inline script validates the ID format), so local `pnpm dev`
# never loads GA. Substitution always starts from the pristine template so
# container restarts with a changed GA_ID work correctly.
#
# GA_ID semantics (this is an open-source project — no measurement ID is
# baked into the image; the operator supplies it at runtime):
#   unset or empty -> GA disabled (ID substituted as empty, JS guard no-ops)
#   set            -> that ID
set -e

TEMPLATE=/app/index.html.tpl
TARGET=/usr/share/nginx/html/index.html

GA_ID="${GA_ID-}"
# Sanitize: GA measurement IDs are alphanumeric plus dashes. Stripping
# anything else prevents a hostile value from breaking out of the JS string.
GA_ID=$(printf '%s' "$GA_ID" | tr -cd 'A-Za-z0-9-')

if [ -f "$TEMPLATE" ]; then
    sed "s|__GA_ID__|${GA_ID}|g" "$TEMPLATE" > "$TARGET"
fi

exec /usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf
