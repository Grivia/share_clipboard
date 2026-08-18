# FastCopy

FastCopy is an end-to-end encrypted clipboard synchronizer. This repository
contains the cross-platform service and clients:

- `server/`: Go API, PostgreSQL persistence, and WebSocket presence/events.
- `macos/`: a minimal macOS menu-bar client.
- `android-kernelsu/`: a KernelSU module with a native daemon and Android
  clipboard bridge.
- `windows/`: a native .NET 8 notification-area client for Windows 10 and 11.
- `shared/`: protocol documentation shared by all clients.

The server never receives clipboard plaintext. After authentication, each
client derives the same 256-bit AES-GCM key locally from the canonical account
and password, then stores it in private device storage.

See each component's README for build and configuration instructions.

## Current deployment

The production API is available at `https://zhy.hair/fastcopy`. Its health
endpoint is `https://zhy.hair/fastcopy/healthz`. The deployment lives under
`/Volumes/SSD_ZHITAI/my-cloudflared-app/fastcopy` and is connected to the
existing Nginx and Cloudflare Tunnel stack.

Build artifacts:

- macOS: `macos/dist/FastCopy.app` and `macos/dist/FastCopy-macos-v0.2.1.zip`
- KernelSU arm64: `android-kernelsu/dist/fastcopy-kernelsu-arm64-v0.3.3.zip`
- Windows x64: `windows/dist/win-x64/FastCopy.exe` and the corresponding zip

The production server accepts one account. Submit the account and password on
any client: the first request creates the account, and later requests sign in.
