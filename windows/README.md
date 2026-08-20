# 粘贴板助手 Windows 客户端

粘贴板助手 for Windows is a small native tray client for Windows 10 and 11. It
uses the existing v1 API and synchronizes text clipboard contents with
the macOS and Android clients.

## Behavior

- The first launch opens a compact login-or-register window.
- After login, closing the window leaves 粘贴板助手 in the notification area.
- A system clipboard event is used for local changes; there is no clipboard
  polling loop.
- A contiguous encrypted WebSocket event is decrypted and applied directly.
  Sequence gaps and reconnects use REST cursor recovery. The safety
  reconciliation runs every five minutes while connected and every minute
  while disconnected.
- The device list is refreshed when the settings window is opened, when the
  user clicks Refresh, or when a presence event arrives while the window is
  visible.
- The device tab shows super-admin, admin, and member roles and exposes only
  the management actions permitted by the server.
- Only text is synchronized. Files and images are ignored.

The default server is `https://zhy.hair/fastcopy`. Accounts do not need to be
email addresses. A missing account is created by the same login action.

## Security and local data

Clipboard text is encrypted locally with the shared AES-256-GCM protocol before
upload. Access tokens, refresh tokens, and the derived clipboard key are stored
with Windows DPAPI for the current Windows user. Pending uploads are stored only
as encrypted envelopes. Plain clipboard text is never written to disk by this
client.

For upgrade compatibility, local state and logs remain in the internal
`%LOCALAPPDATA%\FastCopy` directory.

## Build

Install the .NET 8 SDK, then run in PowerShell:

```powershell
cd windows
.\build.ps1
```

The script runs the protocol smoke tests and creates a self-contained,
single-file x64 build at
`dist\clipboard-assistant-win-x64\ClipboardAssistant.exe`, plus a zip archive.
An ARM64 build can be produced with:

```powershell
.\build.ps1 -Runtime win-arm64
```

The self-contained executable does not require a separate .NET installation.
Code signing is not part of this repository, so Windows SmartScreen can warn
when an unsigned build is downloaded on another computer.
