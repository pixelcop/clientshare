#!/bin/sh
set -eu

# Executed by Nginx's entrypoint on every container start. jq handles JSON escaping
# so labels and URLs never become executable JavaScript or unescaped HTML.
enabled="${DOCS_SITE_LINK_ENABLED:-true}"
case "$enabled" in
    true|false) ;;
    *) printf '%s\n' 'DOCS_SITE_LINK_ENABLED must be true or false' >&2; exit 1 ;;
esac

jq -n \
    --argjson enabled "$enabled" \
    --arg url "${DOCS_SITE_LINK_URL:-}" \
    --arg label "${DOCS_SITE_LINK_LABEL:-}" '
    def trim: gsub("^\\s+|\\s+$"; "");
    ($url | trim) as $url |
    ($label | trim) as $label |
    if ($enabled | not) or $url == "" then
        {site_link: null}
    elif ($url | test("[\\\\\\r\\n\\t]")) or
         ((($url | test("^https?://[^/?#@\\s]+([/?#]|$)|^/[^/]")) | not) and $url != "/") then
        error("DOCS_SITE_LINK_URL must be an http(s) URL or a root-relative path")
    else
        {site_link: {url: $url, label: (if $label == "" then "Website" else $label end)}}
    end
    ' > /usr/share/nginx/html/site-config.json
