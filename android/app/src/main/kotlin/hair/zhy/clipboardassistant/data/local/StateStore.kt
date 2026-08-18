package hair.zhy.clipboardassistant.data.local

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import hair.zhy.clipboardassistant.data.model.PersistedState
import java.util.UUID
import kotlinx.coroutines.flow.first
import kotlinx.serialization.json.Json

private val Context.clipboardStateDataStore by preferencesDataStore(name = "clipboard_state")

class StateStore(private val context: Context, private val json: Json) {
    private val stateKey = stringPreferencesKey("state_v1")

    suspend fun load(): PersistedState {
        val encoded = context.clipboardStateDataStore.data.first()[stateKey]
        if (encoded != null) {
            runCatching { json.decodeFromString<PersistedState>(encoded) }.getOrNull()?.let { return it }
        }
        val initial = PersistedState(installationId = UUID.randomUUID().toString())
        save(initial)
        return initial
    }

    suspend fun save(state: PersistedState) {
        context.clipboardStateDataStore.edit { preferences ->
            preferences[stateKey] = json.encodeToString(state)
        }
    }
}
