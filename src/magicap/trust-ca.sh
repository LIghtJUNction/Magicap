#!/system/bin/sh
# Preserve the active platform anchors, then add this module's public CA.
set -eu
MODDIR=${0%/*}
STATE=${MAGICAP_STATE:-/data/adb/magicap}
APEX=/apex/com.android.conscrypt/cacerts
STORE="$STATE/trust/cacerts"
MARK="$STATE/trust/enabled"

mount_for_pid() {
    pid=$1
    [ -r "/proc/$pid/ns/mnt" ] || return 0
    if grep -q '/magicap/trust/cacerts /apex/com.android.conscrypt/cacerts ' "/proc/$pid/mountinfo"; then return 0; fi
    nsenter --mount="/proc/$pid/ns/mnt" -- mount --bind "$STORE" "$APEX"
    nsenter --mount="/proc/$pid/ns/mnt" -- test -r "$APEX/$CERT_NAME"
}

case "${1:-status}" in
    enable|boot)
        [ "${1:-}" != boot ] || [ -f "$MARK" ] || exit 0
        [ -d "$APEX" ] || { printf '%s\n' 'Conscrypt APEX store is absent; system overlay remains in use.'; exit 0; }
        CERT_NAME=$(find "$MODDIR/system/etc/security/cacerts" -maxdepth 1 -type f -name '*.0' | head -n 1)
        [ -n "$CERT_NAME" ] || { printf '%s\n' 'Module CA overlay is missing' >&2; exit 1; }
        CERT_NAME=${CERT_NAME##*/}
        [ -r "$STATE/certs/ca.pem" ] || exit 1
        mkdir -p "$STATE/trust"
        chmod 700 "$STATE/trust"
        mkdir -p "$STORE"
        chmod 755 "$STORE"
        # Copy rather than replace the current store: retain certificates from other modules.
        if ! grep -q '/magicap/trust/cacerts /apex/com.android.conscrypt/cacerts ' /proc/self/mountinfo; then
            cp "$APEX/"* "$STORE/"
        fi
        cp "$STATE/certs/ca.pem" "$STORE/$CERT_NAME"
        [ "$(find "$STORE" -maxdepth 1 -type f | wc -l)" -gt 5 ] || exit 1
        chown -R 0:0 "$STORE"
        chmod 644 "$STORE/"*
        chcon -R u:object_r:system_security_cacerts_file:s0 "$STORE"
        for pid in 1 $(pidof zygote zygote64 2>/dev/null || true); do
            mount_for_pid "$pid"
        done
        : > "$MARK"
        chmod 600 "$MARK"
        printf '%s\n' "CA mounted in platform/zygote trust stores: $CERT_NAME"
        ;;
    disable)
        for entry in /proc/[0-9]*/mountinfo; do
            grep -q '/magicap/trust/cacerts /apex/com.android.conscrypt/cacerts ' "$entry" 2>/dev/null || continue
            pid=${entry#/proc/}; pid=${pid%/mountinfo}
            nsenter --mount="/proc/$pid/ns/mnt" -- umount "$APEX" 2>/dev/null || true
        done
        [ ! -f "$MARK" ] || rm "$MARK"
        ;;
    pid)
        [ "$#" -eq 2 ] || exit 1
        case "$2" in ''|*[!0-9]*) exit 1 ;; esac
        CERT_NAME=$(find "$MODDIR/system/etc/security/cacerts" -maxdepth 1 -type f -name '*.0' | head -n 1)
        CERT_NAME=${CERT_NAME##*/}
        mount_for_pid "$2"
        ;;
    app)
        CERT_NAME=$(find "$MODDIR/system/etc/security/cacerts" -maxdepth 1 -type f -name '*.0' | head -n 1)
        CERT_NAME=${CERT_NAME##*/}
        [ "$#" -eq 2 ] || exit 1
        for pid in $(pidof "$2" 2>/dev/null || true); do mount_for_pid "$pid"; done
        ;;
    status)
        [ -f "$MARK" ] && printf '%s\n' 'enabled' || printf '%s\n' 'disabled'
        ;;
    *) printf '%s\n' 'Usage: trust-ca.sh enable|disable|status|boot|app PACKAGE' >&2; exit 1 ;;
esac
