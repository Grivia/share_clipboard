package hair.zhy.clipboardassistant

import android.Manifest
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import hair.zhy.clipboardassistant.ui.AppViewModel
import hair.zhy.clipboardassistant.ui.ClipboardAssistantRoot
import hair.zhy.clipboardassistant.ui.theme.ClipboardAssistantTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val repository = (application as ClipboardAssistantApplication).container.repository
        setContent {
            val model: AppViewModel = viewModel(factory = AppViewModel.factory(repository))
            val state by model.state.collectAsStateWithLifecycle()
            val lifecycleOwner = LocalLifecycleOwner.current
            val notificationPermission = rememberLauncherForActivityResult(
                ActivityResultContracts.RequestPermission(),
            ) {}

            DisposableEffect(lifecycleOwner) {
                val observer = LifecycleEventObserver { _, event ->
                    when (event) {
                        Lifecycle.Event.ON_START -> model.setForeground(true)
                        Lifecycle.Event.ON_STOP -> model.setForeground(false)
                        else -> Unit
                    }
                }
                lifecycleOwner.lifecycle.addObserver(observer)
                onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
            }

            LaunchedEffect(state.authenticated) {
                if (state.authenticated && Build.VERSION.SDK_INT >= 33) {
                    notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
                }
            }

            ClipboardAssistantTheme {
                ClipboardAssistantRoot(state = state, model = model)
            }
        }
    }
}
