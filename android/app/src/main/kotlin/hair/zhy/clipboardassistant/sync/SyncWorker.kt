package hair.zhy.clipboardassistant.sync

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import hair.zhy.clipboardassistant.ClipboardAssistantApplication

class SyncWorker(
    appContext: Context,
    workerParameters: WorkerParameters,
) : CoroutineWorker(appContext, workerParameters) {
    override suspend fun doWork(): Result {
        val app = applicationContext as ClipboardAssistantApplication
        return if (app.container.repository.backgroundSync()) Result.success() else Result.retry()
    }
}
