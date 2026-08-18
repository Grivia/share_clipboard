# 粘贴板助手 Android 客户端

普通 Android 客户端使用单 Activity、Jetpack Compose Material 3、ViewModel/UDF、DataStore、WorkManager、Kotlin Coroutines/Flow 和 Repository 分层架构。登录状态和端到端密钥由 Android Keystore 保护，待发送队列只持久化加密信封。

Android 10 起，普通后台应用不能读取其他前台应用写入的剪贴板。因此本客户端在可见时监听并自动同步；后台由 WorkManager 定期补拉远端密文，收到内容后显示系统通知，用户打开应用后才写入剪贴板。需要始终自动读写剪贴板的 Root 设备应使用 `android-kernelsu/` 模块。

## 构建

要求 JDK 17 或更高版本、Android SDK 36 和 Build Tools 36.1.0：

```bash
./gradlew :app:testDebugUnitTest :app:assembleDebug
```

APK 输出到：

```text
app/build/outputs/apk/debug/app-debug.apk
```

客户端暂时不接入 FCM 或其他 Android Push。前台使用 WebSocket，后台使用 WorkManager 游标补拉。

调试 APK 使用 Android SDK 自动生成的 debug key 签名，只适合开发测试。正式发布时应在本地或 CI 中提供 release keystore，并按 Google Play 的要求生成 AAB。
