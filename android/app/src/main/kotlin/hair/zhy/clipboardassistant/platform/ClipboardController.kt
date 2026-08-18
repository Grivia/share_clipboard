package hair.zhy.clipboardassistant.platform

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context

class ClipboardController(context: Context) {
    private val clipboard = context.getSystemService(ClipboardManager::class.java)
    private var listener: ClipboardManager.OnPrimaryClipChangedListener? = null
    private var lastText: String? = null

    fun readText(): String? {
        val data = clipboard.primaryClip ?: return null
        if (data.itemCount == 0) return null
        return data.getItemAt(0).text?.toString()
    }

    fun start(onTextChanged: (String) -> Unit) {
        stop()
        lastText = readText()
        val newListener = ClipboardManager.OnPrimaryClipChangedListener {
            val text = readText() ?: return@OnPrimaryClipChangedListener
            if (text == lastText) return@OnPrimaryClipChangedListener
            lastText = text
            onTextChanged(text)
        }
        clipboard.addPrimaryClipChangedListener(newListener)
        listener = newListener
    }

    fun stop() {
        listener?.let(clipboard::removePrimaryClipChangedListener)
        listener = null
    }

    fun writeRemote(text: String) {
        lastText = text
        clipboard.setPrimaryClip(ClipData.newPlainText("粘贴板助手", text))
    }
}
