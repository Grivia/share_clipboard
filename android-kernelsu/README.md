# 粘贴板助手 KernelSU 模块

This is an arm64 KernelSU module for Android 10 and newer. A Go daemon owns the
network connection, encrypted retry queue, and cursor recovery. A small Java
class started with `app_process` under the Android shell identity bridges the
daemon to `ClipboardManager`. The same bridge delegates hostname resolution to
Android's native resolver, avoiding the pure-Go Android DNS limitation.

Build the flashable ZIP on macOS:

```bash
chmod +x scripts/build-module.sh
./scripts/build-module.sh
```

Install `dist/clipboard-assistant-kernelsu-arm64-v0.3.8.zip` from KernelSU Manager, reboot,
then open the module WebUI. Enter the same account and password used by the
macOS client. A missing account is registered automatically.

After authentication succeeds, the WebUI removes the account and password
fields and shows historical devices, presence, and device roles. A super-admin
module can grant or remove admin access; super admins and admins can force
permitted devices offline. The device list is fetched when the WebUI opens,
after an action, or when its refresh button is pressed; the background
synchronization loop does not poll the device endpoint. The account section
also provides a sign-out button that revokes the server session and removes
local tokens, the encryption key, and pending uploads.

The WebUI defaults to the production API at `https://zhy.hair/fastcopy`.
Submitting credentials automatically enables synchronization and starts the
sign-in attempt. After sign-in, the same switch can pause background syncing.

Runtime controls are also available over adb:

```bash
su -c /data/adb/modules/fastcopy_kernelsu/bin/fastcopyctl status
su -c /data/adb/modules/fastcopy_kernelsu/bin/fastcopyctl restart
su -c /data/adb/modules/fastcopy_kernelsu/bin/fastcopyctl logs
su -c /data/adb/modules/fastcopy_kernelsu/bin/fastcopyctl logout
```

Persistent private state is stored with mode `0600` under `/data/adb/fastcopy`.
Module settings are stored in `/data/adb/fastcopy/settings.json`; the module
does not depend on the newer `ksud module config` command and has been verified
with ksud 0.9.5.
The account password is removed from module configuration after successful
authentication. The locally derived encryption key and session tokens are
kept in private runtime state and never appear in the WebUI configuration.

The clipboard bridge relies on the privileged `com.android.shell` package's
background clipboard permission. Version 0.3.3 discovers packages belonging to
the process UID at runtime and probes multiple context and ClipboardManager
strategies instead of hardcoding one OEM-specific path. It starts with the
least-privileged shell identity and can fall back to an isolated system-UID
bridge if a ROM removes shell clipboard access. The network daemon remains in
its original root process and never handles clipboard Binder calls directly.

The expected compatibility range is arm64 Android 10 through Android 16 on
AOSP-derived ROMs. It has been physically verified on MIUI Android 13 with
KernelSU 0.9.5. Other ROMs remain expected-compatible rather than verified
until tested on real hardware. The bridge currently targets the primary Android
user (user 0) and synchronizes plain text only.

Some ROMs hide clipboard contents or silently ignore clipboard writes while the
screen is locked. Every remote write is therefore read back and acknowledged by
the bridge. If verification fails, the server cursor is left unchanged and the
daemon reports `waiting_unlock` until it can retry after the device is unlocked.

While WebSocket is connected and synchronization is healthy, contiguous
encrypted events are decrypted and applied directly. Sequence gaps and
reconnects use REST cursor recovery, with a five-minute safety reconciliation.
Disconnected operation falls back to a 30-second reconciliation, network
failures use 2/5/15/30/60-second backoff, and locked-screen clipboard writes
retry every 10 seconds.

Some MIUI Android 13 builds print a missing
`/data/system/theme_config/theme_compatibility.xml` stack trace while
`app_process` initializes system resources. It is non-fatal when the following
lines show a UID/package pair, a selected strategy, and `READY`. A message such
as `Package android does not belong to 2000` indicates the old bridge from
version 0.2.1 or earlier and requires upgrading the module.
