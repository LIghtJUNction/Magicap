#!/system/bin/sh
MODDIR=${0%/*}
# Retain CA identity and captures outside the module; never delete user data implicitly.
"$MODDIR/cli" stop

/system/bin/sh "$MODDIR/app-routes.sh" clear
/system/bin/sh "$MODDIR/trust-ca.sh" disable
