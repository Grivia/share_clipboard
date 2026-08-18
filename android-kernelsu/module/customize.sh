#!/system/bin/sh

DEVICE_ARCH=${ARCH:-$(getprop ro.product.cpu.abi)}
case "$DEVICE_ARCH" in
    arm64|arm64-v8a)
        ;;
    *)
        abort "FastCopy currently supports arm64 devices only"
        ;;
esac

DEVICE_API=${API:-$(getprop ro.build.version.sdk)}
if [ -z "$DEVICE_API" ] || [ "$DEVICE_API" -lt 29 ]; then
    abort "FastCopy requires Android 10 (API 29) or newer"
fi

set_perm "$MODPATH/service.sh" 0 0 0755
set_perm "$MODPATH/action.sh" 0 0 0755
set_perm "$MODPATH/uninstall.sh" 0 0 0755
set_perm "$MODPATH/bin/fastcopyctl" 0 0 0755
set_perm "$MODPATH/bin/fastcopyd" 0 0 0755
set_perm "$MODPATH/bin/fastcopy-bridge.jar" 0 0 0644

mkdir -p /data/adb/fastcopy
chmod 0700 /data/adb/fastcopy
