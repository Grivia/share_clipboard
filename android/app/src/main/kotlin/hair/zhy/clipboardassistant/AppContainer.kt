package hair.zhy.clipboardassistant

import android.content.Context
import hair.zhy.clipboardassistant.data.ClipboardRepository
import hair.zhy.clipboardassistant.data.local.SecretStore
import hair.zhy.clipboardassistant.data.local.StateStore
import hair.zhy.clipboardassistant.data.remote.ApiClient
import hair.zhy.clipboardassistant.platform.ClipboardController
import hair.zhy.clipboardassistant.platform.NotificationController
import hair.zhy.clipboardassistant.sync.SyncScheduler
import java.util.concurrent.TimeUnit
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient

class AppContainer(context: Context) {
    private val appContext = context.applicationContext
    private val json = Json {
        ignoreUnknownKeys = true
        encodeDefaults = true
        explicitNulls = false
    }
    private val http = OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .pingInterval(45, TimeUnit.SECONDS)
        .build()
    private val notifications = NotificationController(appContext)

    val repository = ClipboardRepository(
        stateStore = StateStore(appContext, json),
        secretStore = SecretStore(appContext, json),
        apiFactory = { serverUrl -> ApiClient(serverUrl, http, json) },
        clipboard = ClipboardController(appContext),
        notifications = notifications,
        scheduler = SyncScheduler(appContext),
    )

    init {
        notifications.createChannel()
    }
}
