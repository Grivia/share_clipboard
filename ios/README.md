# 粘贴板助手 iOS 客户端

iOS 客户端采用 SwiftUI、async/await、URLSession WebSocket、Keychain 和
UserNotifications。界面使用原生 `NavigationStack`、`List`、`Form` 与
`TabView`，最低系统版本为 iOS 17。

设备列表显示三级设备角色，并通过原生滑动操作提供服务端允许的管理员设置和设备下线功能。只有账号的超级管理员设备可以授予或撤销管理员。

## 同步行为

- 应用位于前台时监听系统剪贴板，并通过 WebSocket 接收即时事件。
- 用户也可以手动发送当前剪贴板或复制最近一次远端内容。
- 应用离线时由 APNs 发送提醒；通知只包含事件序号等元数据。
- 收到后台通知后，应用通过 HTTPS 拉取端到端加密信封并更新本地游标。
- iOS 不允许本应用长期在后台任意监控剪贴板，因此后台不会直接改写
  `UIPasteboard`；应用再次进入前台后才应用已经拉取的内容。

会话令牌和派生密钥保存在 Keychain，游标、最新远端事件和待上传队列
保存在 UserDefaults，其中剪贴板内容均为 AES-256-GCM 密文。

## 构建与测试

仓库已经提交生成后的 `ClipboardAssistant.xcodeproj`，可以直接用 Xcode
打开。若修改 `project.yml`，先安装 XcodeGen 并重新生成工程：

```bash
cd ios
xcodegen generate
open ClipboardAssistant.xcodeproj
```

命令行构建：

```bash
xcodebuild \
  -project ClipboardAssistant.xcodeproj \
  -scheme ClipboardAssistant \
  -sdk iphonesimulator \
  -destination 'generic/platform=iOS Simulator' \
  CODE_SIGNING_ALLOWED=NO build
```

测试覆盖 PBKDF2 协议向量和 Unicode 文本加密往返。选择一个本机模拟器后运行：

```bash
xcodebuild \
  -project ClipboardAssistant.xcodeproj \
  -scheme ClipboardAssistant \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  test
```

## APNs 配置

1. 在 Apple Developer 中为 `hair.zhy.fastcopy.ios` 创建 App ID。
2. 启用 Push Notifications capability，并创建 APNs token signing key。
3. 在 Xcode 的 Signing & Capabilities 中选择自己的 Team。项目已经声明
   Push Notifications entitlement 和 `remote-notification` 后台模式。
4. 把 `.p8` 文件作为只读 secret 挂载给服务端，并填写服务端 README 中的
   `FASTCOPY_APNS_*` 环境变量。

Debug 构建上传 `sandbox` token，Release 构建上传 `production` token。APNs
device token 由系统在每次启动时重新提供，客户端不把它永久缓存。模拟器可
验证 UI 与加密逻辑，但完整推送链路应在已签名真机上测试。

APNs 后台通知由系统调度，不保证每次都立即执行。服务端事件历史和本地游标
用于补偿未送达或延迟的通知，因此 Push 不是可靠数据通道。

## App Store 隐私信息

`Resources/PrivacyInfo.xcprivacy` 已声明应用自身使用 UserDefaults 的
`CA92.1` 理由，并声明为了 App Functionality 收集且关联账号的设备名称、
用户 ID、设备 ID 和用户内容；不用于 Tracking。剪贴板正文虽然端到端加密，
仍按用户内容保守申报。App Store Connect 中填写的 Privacy Nutrition Label
应与此清单和实际服务端行为保持一致。
