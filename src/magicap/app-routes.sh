#!/system/bin/sh
set -eu
NAT=MAGICAP_OUT
FILTER=MAGICAP_QUIC
clear_routes() {
    for tool in iptables ip6tables; do
        command -v "$tool" >/dev/null 2>&1 || continue
        while "$tool" -w 3 -t nat -C OUTPUT -j "$NAT" 2>/dev/null; do "$tool" -w 3 -t nat -D OUTPUT -j "$NAT"; done
        if "$tool" -w 3 -t nat -S "$NAT" >/dev/null 2>&1; then "$tool" -w 3 -t nat -F "$NAT"; "$tool" -w 3 -t nat -X "$NAT"; fi
        while "$tool" -w 3 -t filter -C OUTPUT -j "$FILTER" 2>/dev/null; do "$tool" -w 3 -t filter -D OUTPUT -j "$FILTER"; done
        if "$tool" -w 3 -t filter -S "$FILTER" >/dev/null 2>&1; then "$tool" -w 3 -t filter -F "$FILTER"; "$tool" -w 3 -t filter -X "$FILTER"; fi
    done
}
case "${1:-status}" in
    apply)
        shift; TLS_PORT=$1; HTTP_PORT=$2; shift 2
        case "$TLS_PORT:$HTTP_PORT" in *[!0-9:]*|'') exit 1 ;; esac
        [ "$TLS_PORT" -ge 1024 ] && [ "$TLS_PORT" -le 65535 ]
        [ "$HTTP_PORT" -ge 1024 ] && [ "$HTTP_PORT" -le 65535 ]
        for uid in "$@"; do case "$uid" in ''|*[!0-9]*) exit 1 ;; esac; [ "$uid" -ge 10000 ] || exit 1; done
        clear_routes
        [ "$#" -gt 0 ] || exit 0
        trap 'clear_routes' EXIT
        for tool in iptables ip6tables; do
            has_nat=1
            if ! "$tool" -w 3 -t nat -S OUTPUT >/dev/null 2>&1; then
                [ "$tool" = ip6tables ] || exit 1
                has_nat=0
            fi
            if [ "$has_nat" -eq 1 ]; then "$tool" -w 3 -t nat -N "$NAT"; fi
            "$tool" -w 3 -t filter -N "$FILTER"
            for uid in "$@"; do
                if [ "$has_nat" -eq 1 ]; then
                    "$tool" -w 3 -t nat -A "$NAT" -m owner --uid-owner "$uid" -p tcp --dport 443 -j REDIRECT --to-ports "$TLS_PORT"
                    "$tool" -w 3 -t nat -A "$NAT" -m owner --uid-owner "$uid" -p tcp --dport 80 -j REDIRECT --to-ports "$HTTP_PORT"
                else
                    # Keep the fallback scoped to selected UIDs on kernels without IPv6 NAT.
                    "$tool" -w 3 -t filter -A "$FILTER" -m owner --uid-owner "$uid" -p tcp -m multiport --dports 80,443 -j REJECT
                fi
                "$tool" -w 3 -t filter -A "$FILTER" -m owner --uid-owner "$uid" -p udp --dport 443 -j REJECT
            done
            if [ "$has_nat" -eq 1 ]; then "$tool" -w 3 -t nat -I OUTPUT 1 -j "$NAT"; fi
            "$tool" -w 3 -t filter -I OUTPUT 1 -j "$FILTER"
        done
        trap - EXIT
        ;;
    clear) clear_routes ;;
    status)
        for tool in iptables ip6tables; do "$tool" -t nat -S "$NAT" 2>/dev/null || true; done
        ;;
    *) printf '%s\n' 'Usage: app-routes.sh apply TLS_PORT HTTP_PORT UID... | clear | status' >&2; exit 1 ;;
esac
