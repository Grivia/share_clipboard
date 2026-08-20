using System.Security.Cryptography;
using System.Text.Json;
using FastCopy.Core;

try
{
    ProtocolKeyDerivationMatchesOtherClients();
    EncryptionRoundTripsUnicodeText();
    AuthenticationDataRejectsTampering();
    JsonUsesProtocolFieldNames();
    WebSocketClipPayloadDeserializes();
    PushedClipSequenceActions();
    RetryIntervalsMatchOtherClients();
    Console.WriteLine("FastCopy.Core smoke tests passed.");
    return 0;
}
catch (Exception exception)
{
    Console.Error.WriteLine(exception);
    return 1;
}

static void ProtocolKeyDerivationMatchesOtherClients()
{
    var key = FastCopyCrypto.DeriveKey("alice", "correct horse battery staple");
    try
    {
        Equal(
            "dpMRWwaHgnInWXwAZC2TxG3GuJZGNbWhYCGNP5T6I2g=",
            Convert.ToBase64String(key),
            "PBKDF2 protocol vector");
    }
    finally
    {
        CryptographicOperations.ZeroMemory(key);
    }
}

static void EncryptionRoundTripsUnicodeText()
{
    var key = RandomNumberGenerator.GetBytes(32);
    try
    {
        const string eventId = "8b949ec8-42f0-4a71-8fb3-20f094e64f4a";
        var upload = FastCopyCrypto.Encrypt("跨设备文本\r\nFastCopy", key, eventId);
        Equal("text/plain", upload.ContentType, "content type");
        Equal("AES-256-GCM", upload.Algorithm, "algorithm");
        Equal(12, Convert.FromBase64String(upload.Nonce).Length, "nonce length");

        var clip = ToClip(upload);
        Equal("跨设备文本\r\nFastCopy", FastCopyCrypto.Decrypt(clip, key), "round trip");
    }
    finally
    {
        CryptographicOperations.ZeroMemory(key);
    }
}

static void AuthenticationDataRejectsTampering()
{
    var key = RandomNumberGenerator.GetBytes(32);
    try
    {
        var upload = FastCopyCrypto.Encrypt(
            "secret",
            key,
            "3db57c16-2845-467b-9efe-c3b49161637a");
        var encrypted = Convert.FromBase64String(upload.Ciphertext);
        encrypted[^1] ^= 0x01;
        var tampered = upload with { Ciphertext = Convert.ToBase64String(encrypted) };
        Throws<CryptographicException>(
            () => FastCopyCrypto.Decrypt(ToClip(tampered), key),
            "tampered ciphertext");
    }
    finally
    {
        CryptographicOperations.ZeroMemory(key);
    }
}

static void JsonUsesProtocolFieldNames()
{
    var upload = new ClipUpload("event", "text/plain", "AES-256-GCM", "nonce", "ciphertext");
    var json = JsonSerializer.Serialize(upload, FastCopyJson.Options);
    True(json.Contains("\"client_event_id\"", StringComparison.Ordinal), "snake_case JSON");
    True(!json.Contains("ClientEventId", StringComparison.Ordinal), "no CLR field names in JSON");
}

static void WebSocketClipPayloadDeserializes()
{
    const string json = """
        {"type":"clip.created","data":{"event_id":"server-event","client_event_id":"client-event","seq":8,"origin_device_id":"device-a","origin_name":"Windows","content_type":"text/plain","algorithm":"AES-256-GCM","nonce":"nonce","ciphertext":"ciphertext","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-02T00:00:00Z"}}
        """;
    var envelope = JsonSerializer.Deserialize<WebSocketEnvelope>(json, FastCopyJson.Options)
        ?? throw new InvalidOperationException("WebSocket envelope was null");
    var clip = envelope.Data?.Deserialize<ClipEvent>(FastCopyJson.Options)
        ?? throw new InvalidOperationException("WebSocket clip payload was null");
    Equal("clip.created", envelope.Type, "WebSocket event type");
    Equal(8L, clip.Seq, "WebSocket clip sequence");
    Equal("device-a", clip.OriginDeviceId, "WebSocket origin device");
}

static void PushedClipSequenceActions()
{
    Equal(PushedClipAction.Ignore, ClipSequence.Action(7, 7), "same sequence");
    Equal(PushedClipAction.Ignore, ClipSequence.Action(7, 6), "older sequence");
    Equal(PushedClipAction.Apply, ClipSequence.Action(7, 8), "next sequence");
    Equal(PushedClipAction.Reconcile, ClipSequence.Action(7, 9), "sequence gap");
}

static void RetryIntervalsMatchOtherClients()
{
    var expected = new[] { 2, 5, 15, 30, 60, 60 };
    for (var index = 0; index < expected.Length; index++)
    {
        Equal(TimeSpan.FromSeconds(expected[index]), SyncTiming.PendingRetry(index), $"retry {index}");
    }
    Equal(TimeSpan.FromMinutes(5), SyncTiming.ConnectedReconciliation, "connected reconciliation");
    Equal(TimeSpan.FromMinutes(1), SyncTiming.DisconnectedReconciliation, "offline reconciliation");
}

static ClipEvent ToClip(ClipUpload upload) => new(
    "server-event",
    upload.ClientEventId,
    1,
    "device-a",
    "Test device",
    upload.ContentType,
    upload.Algorithm,
    upload.Nonce,
    upload.Ciphertext,
    "2026-01-01T00:00:00Z",
    "2026-01-02T00:00:00Z");

static void Equal<T>(T expected, T actual, string name)
{
    if (!EqualityComparer<T>.Default.Equals(expected, actual))
    {
        throw new InvalidOperationException($"{name}: expected {expected}, got {actual}");
    }
}

static void True(bool value, string name)
{
    if (!value)
    {
        throw new InvalidOperationException($"{name}: assertion failed");
    }
}

static void Throws<T>(Action action, string name) where T : Exception
{
    try
    {
        action();
    }
    catch (T)
    {
        return;
    }
    throw new InvalidOperationException($"{name}: expected {typeof(T).Name}");
}
