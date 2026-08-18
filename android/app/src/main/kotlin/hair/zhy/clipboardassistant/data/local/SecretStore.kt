package hair.zhy.clipboardassistant.data.local

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.core.content.edit
import hair.zhy.clipboardassistant.data.model.SecretState
import java.security.KeyStore
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import kotlinx.serialization.json.Json

class SecretStore(context: Context, private val json: Json) {
    private val preferences = context.getSharedPreferences("protected_session", Context.MODE_PRIVATE)
    private val alias = "clipboard_assistant_session_v1"
    private val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    fun load(): SecretState? {
        val encoded = preferences.getString("session", null) ?: return null
        return runCatching {
            val value = Base64.getDecoder().decode(encoded)
            require(value.size > 12)
            val nonce = value.copyOfRange(0, 12)
            val ciphertext = value.copyOfRange(12, value.size)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, secretKey(), GCMParameterSpec(128, nonce))
            json.decodeFromString<SecretState>(cipher.doFinal(ciphertext).decodeToString())
        }.getOrNull()
    }

    fun save(state: SecretState) {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, secretKey())
        val ciphertext = cipher.doFinal(json.encodeToString(state).encodeToByteArray())
        val value = cipher.iv + ciphertext
        preferences.edit { putString("session", Base64.getEncoder().encodeToString(value)) }
    }

    fun clear() {
        preferences.edit { remove("session") }
    }

    private fun secretKey(): SecretKey {
        (keyStore.getKey(alias, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        generator.init(
            KeyGenParameterSpec.Builder(
                alias,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return generator.generateKey()
    }
}
