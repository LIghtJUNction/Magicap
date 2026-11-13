#!/system/bin/sh
MODDIR=${0%/*}
exec "$MODDIR/cli" status
