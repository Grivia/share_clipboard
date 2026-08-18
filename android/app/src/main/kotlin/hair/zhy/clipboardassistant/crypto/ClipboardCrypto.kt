package hair.zhy.clipboardassistant.crypto

import hair.zhy.clipboardassistant.data.model.ClipEvent
import hair.zhy.clipboardassistant.data.model.ClipUpload
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.PBEKeySpec
import javax.crypto.spec.SecretKeySpec

object ClipboardCrypto {
    const val MaxPlaintextBytes = 256 * 1024 - 16
    private const val Iterations = 210_000
    private const val KeyBits = 256
    private val random = SecureRandom()

    fun deriveKey(account: String, password: String): ByteArray {
        val saltSource = "fastcopy:key-salt:v1|$account".toByteArray(StandardCharsets.UTF_8)
        val salt = MessageDigest.getInstance("SHA-256").digest(saltSource)
        val spec = PBEKeySpec(password.toCharArray(), salt, Iterations, KeyBits)
        return try {
            SecretKeyFactory.getInstance("PBKDF2WithHmacSHA256").generateSecret(spec).encoded
        } finally {
            spec.clearPassword()
        }
    }

    fun encrypt(text: String, key: ByteArray, eventId: String = UUID.randomUUID().toString()): ClipUpload {
        val plaintext = text.toByteArray(StandardCharsets.UTF_8)
        require(plaintext.size <= MaxPlaintextBytes) { "剪贴板文本超过 256 KiB" }
        val nonce = ByteArray(12).also(random::nextBytes)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, nonce))
        cipher.updateAAD(aad(eventId))
        val ciphertext = cipher.doFinal(plaintext)
        return ClipUpload(
            clientEventId = eventId,
            nonce = Base64.getEncoder().encodeToString(nonce),
            ciphertext = Base64.getEncoder().encodeToString(ciphertext),
        )
    }

    fun decrypt(event: ClipEvent, key: ByteArray): String {
        require(event.contentType == "text/plain" && event.algorithm == "AES-256-GCM")
        val nonce = Base64.getDecoder().decode(event.nonce)
        val ciphertext = Base64.getDecoder().decode(event.ciphertext)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, nonce))
        cipher.updateAAD(aad(event.clientEventId))
        return cipher.doFinal(ciphertext).toString(StandardCharsets.UTF_8)
    }

    private fun aad(eventId: String): ByteArray =
        "fastcopy:v1|$eventId|text/plain".toByteArray(StandardCharsets.UTF_8)
}
