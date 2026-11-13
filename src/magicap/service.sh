#!/system/bin/sh
MODDIR=${0%/*}
export MODDIR
. "$MODDIR/lib/kamfw/.kamfwrc" || exit 1
kamfw_phase_service() {
    "$MODDIR/cli" cert overlay || return 1
    "$MODDIR/cli" boot
}
kamfw run service -- "$@"
