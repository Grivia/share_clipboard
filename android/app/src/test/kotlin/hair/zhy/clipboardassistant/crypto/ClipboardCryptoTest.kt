package hair.zhy.clipboardassistant.crypto

import hair.zhy.clipboardassistant.data.model.ClipEvent
import java.util.Base64
import org.junit.Assert.assertEquals
import org.junit.Test

class ClipboardCryptoTest {
    @Test
    fun keyDerivationMatchesProtocolVector() {
        val key = ClipboardCrypto.deriveKey("alice", "correct horse battery staple")
        assertEquals(
            "dpMRWwaHgnInWXwAZC2TxG3GuJZGNbWhYCGNP5T6I2g=",
            Base64.getEncoder().encodeToString(key),
        )
    }

    @Test
    fun encryptionRoundTripsUnicodeText() {
        val key = ClipboardCrypto.deriveKey("测试", "安全 密码")
        val upload = ClipboardCrypto.encrypt("跨设备文本\nAndroid", key)
        val event = ClipEvent(
            eventId = "server-event",
            clientEventId = upload.clientEventId,
            seq = 1,
            originDeviceId = "device-a",
            originName = "Android",
            contentType = upload.contentType,
            algorithm = upload.algorithm,
            nonce = upload.nonce,
            ciphertext = upload.ciphertext,
            createdAt = "2026-01-01T00:00:00Z",
            expiresAt = "2026-01-02T00:00:00Z",
        )
        assertEquals("跨设备文本\nAndroid", ClipboardCrypto.decrypt(event, key))
    }
}
