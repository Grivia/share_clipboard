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

The same command also creates `dist/粘贴板助手-macos-v0.2.6.zip` for transfer to
another Mac.

WebSocket events trigger immediate cursor synchronization. A healthy connection
uses a five-minute safety reconciliation, disconnected operation falls back to
one-minute REST reconciliation, and failed uploads retry with 2/5/15/30/60-second
backoff. Opening the settings window refreshes the device list.

On launch, an unauthenticated client automatically opens and activates the main
window. An authenticated client remains menu-bar-only. If its session expires
later, the same window is brought forward with the sign-in view.

The device list shows each device role. A super-admin device can grant or
remove administrator access; super admins and admins can force permitted
devices offline. Actions are hidden when the server does not grant the matching
capability.

The client has one authentication flow. A new account is registered
automatically, while an existing account is signed in. Access tokens, the
derived encryption key, and the installation ID are encrypted with AES-256-GCM
in `~/Library/Application Support/hair.zhy.fastcopy/credentials.enc`. A random
256-bit key is stored beside it in `credentials.key`; the directory uses mode
`0700` and both files use mode `0600`. This prevents directly readable JSON but
is not a strong boundary against software running as the same macOS user,
because that software may read both the ciphertext and its key.

Version 0.2.5 does not link or call any Keychain API. It converts the local
plaintext `credentials.json` created by 0.2.4 into the encrypted files and then
deletes the plaintext file. It does not import credentials from Keychain, so a
direct upgrade from 0.2.3 or earlier requires signing in again and may create a
new installation identity.

The default production API is `https://zhy.hair/fastcopy`.

The build is ad-hoc signed for local use. A Developer ID signature and notarized
distribution can be added later without changing the client protocol.
