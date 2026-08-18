#!/system/bin/sh

MODDIR=${0%/*}
"$MODDIR/bin/fastcopyctl" restart
sleep 1
"$MODDIR/bin/fastcopyctl" status
