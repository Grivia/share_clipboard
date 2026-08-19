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

The same command also creates `dist/粘贴板助手-macos-v0.2.3.zip` for transfer to
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
automatically, while an existing account is signed in. The encryption key is
derived locally after authentication and stored in Keychain; it is never shown
to the user or sent to the server. The server never receives clipboard
plaintext.

The default production API is `https://zhy.hair/fastcopy`.

The build is ad-hoc signed for local use. A Developer ID signature and notarized
distribution can be added later without changing the client protocol.
