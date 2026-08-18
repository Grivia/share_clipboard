package hair.zhy.clipboardassistant

import android.app.Application

class ClipboardAssistantApplication : Application() {
    val container by lazy { AppContainer(this) }

    override fun onCreate() {
        super.onCreate()
        container
    }
}
