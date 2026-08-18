package hair.zhy.clipboardassistant.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import hair.zhy.clipboardassistant.data.ClipboardRepository
import hair.zhy.clipboardassistant.data.RepositoryState
import hair.zhy.clipboardassistant.data.model.DeviceModel
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

class AppViewModel(private val repository: ClipboardRepository) : ViewModel() {
    val state: StateFlow<RepositoryState> = repository.state

    fun authenticate(serverUrl: String, account: String, password: String) {
        viewModelScope.launch { repository.authenticate(serverUrl, account, password) }
    }

    fun setForeground(foreground: Boolean) = repository.setForeground(foreground)
    fun sendClipboard() = repository.sendCurrentClipboard()
    fun copyLatest() = repository.copyLatest()
    fun refresh() = repository.refreshNow()
    fun setSyncEnabled(enabled: Boolean) = repository.setSyncEnabled(enabled)
    fun refreshDevices() = repository.refreshDevicesAsync()
    fun revokeDevice(device: DeviceModel) = repository.revokeDevice(device)
    fun logout() = repository.logout()

    companion object {
        fun factory(repository: ClipboardRepository): ViewModelProvider.Factory =
            object : ViewModelProvider.Factory {
                @Suppress("UNCHECKED_CAST")
                override fun <T : ViewModel> create(modelClass: Class<T>): T =
                    AppViewModel(repository) as T
            }
    }
}
