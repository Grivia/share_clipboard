#!/system/bin/sh

MODDIR=${0%/*}
"$MODDIR/bin/fastcopyctl" stop
rm -rf /data/adb/fastcopy
rm -f /data/local/tmp/fastcopy-bridge.jar
