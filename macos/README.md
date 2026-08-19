# 粘贴板助手 macOS 客户端

Native macOS 13+ menu-bar client. It polls `NSPasteboard`, encrypts text with
AES-256-GCM, persists encrypted uploads before sending, and uses WebSocket
events as a wake-up signal for cursor-based REST synchronization.

Build the application bundle:

```bash
chmod +x scripts/build-app.sh
./scripts/build-app.sh
open 'dist/粘贴板助手.app'
```

The same command also creates `dist/粘贴板助手-macos-v0.2.4.zip` for transfer to
another Mac.

WebSocket events trigger immediate cursor synchronization. A healthy connection
uses a five-minute safety reconciliation, disconnected operation falls back to
one-minute REST reconciliation, and failed uploads retry with 2/5/15/30/60-second
backoff. Opening the settings window refreshes the device list.

The device list shows each device role. A super-admin device can grant or
remove administrator access; super admins and admins can force permitted
devices offline. Actions are hidden when the server does not grant the matching
capability.

The client has one authentication flow. A new account is registered
automatically, while an existing account is signed in. Access tokens, the
derived encryption key, and the installation ID are stored as plain JSON in
`~/Library/Application Support/hair.zhy.fastcopy/credentials.json`. The file
uses mode `0600`, but applications running as the same macOS user may still
read it. This deliberately trades local security for unattended startup and
avoids recurring Keychain authorization prompts.

Version 0.2.4 performs one legacy Keychain query on its first launch to retain
the existing session and installation identity. After a successful import it
never queries Keychain again. Depending on the old item access rules, macOS may
show one or more final authorization dialogs during this one-time migration.

The default production API is `https://zhy.hair/fastcopy`.

The build is ad-hoc signed for local use. A Developer ID signature and notarized
distribution can be added later without changing the client protocol.
