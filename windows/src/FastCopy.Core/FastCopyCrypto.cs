using System.Security.Cryptography;
using System.Text;

namespace FastCopy.Core;

public static class FastCopyCrypto
{
    public const int KeyDerivationVersion = 1;
    public const int KeyDerivationIterations = 210_000;
    private const string ContentType = "text/plain";

    public static byte[] DeriveKey(string account, string password)
    {
        var saltSource = Encoding.UTF8.GetBytes($"fastcopy:key-salt:v1|{account}");
        var salt = SHA256.HashData(saltSource);
        var passwordBytes = Encoding.UTF8.GetBytes(password);
        try
        {
            return Rfc2898DeriveBytes.Pbkdf2(
                passwordBytes,
                salt,
                KeyDerivationIterations,
                HashAlgorithmName.SHA256,
                32);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(passwordBytes);
            CryptographicOperations.ZeroMemory(salt);
        }
    }

    public static ClipUpload Encrypt(string text, ReadOnlySpan<byte> key, string clientEventId)
    {
        ValidateKey(key);
        var plaintext = Encoding.UTF8.GetBytes(text);
        var nonce = RandomNumberGenerator.GetBytes(12);
        var ciphertext = new byte[plaintext.Length];
        var tag = new byte[16];
        var aad = AdditionalData(clientEventId);
        try
        {
            using var aes = new AesGcm(key, tag.Length);
            aes.Encrypt(nonce, plaintext, ciphertext, tag, aad);
            var encrypted = new byte[ciphertext.Length + tag.Length];
            Buffer.BlockCopy(ciphertext, 0, encrypted, 0, ciphertext.Length);
            Buffer.BlockCopy(tag, 0, encrypted, ciphertext.Length, tag.Length);
            return new ClipUpload(
                clientEventId,
                ContentType,
                "AES-256-GCM",
                Convert.ToBase64String(nonce),
                Convert.ToBase64String(encrypted));
        }
        finally
        {
            CryptographicOperations.ZeroMemory(plaintext);
        }
    }

    public static string Decrypt(ClipEvent clip, ReadOnlySpan<byte> key)
    {
        ValidateKey(key);
        if (clip.ContentType != ContentType || clip.Algorithm != "AES-256-GCM")
        {
            throw new CryptographicException("Unsupported clipboard encryption envelope.");
        }
        var nonce = Convert.FromBase64String(clip.Nonce);
        var encrypted = Convert.FromBase64String(clip.Ciphertext);
        if (nonce.Length != 12 || encrypted.Length < 16)
        {
            throw new CryptographicException("Invalid clipboard encryption envelope.");
        }
        var ciphertextLength = encrypted.Length - 16;
        var plaintext = new byte[ciphertextLength];
        var aad = AdditionalData(clip.ClientEventId);
        using var aes = new AesGcm(key, 16);
        aes.Decrypt(
            nonce,
            encrypted.AsSpan(0, ciphertextLength),
            encrypted.AsSpan(ciphertextLength, 16),
            plaintext,
            aad);
        try
        {
            return new UTF8Encoding(false, true).GetString(plaintext);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(plaintext);
        }
    }

    private static byte[] AdditionalData(string clientEventId) =>
        Encoding.UTF8.GetBytes($"fastcopy:v1|{clientEventId}|{ContentType}");

    private static void ValidateKey(ReadOnlySpan<byte> key)
    {
        if (key.Length != 32)
        {
            throw new CryptographicException("FastCopy key must contain 32 bytes.");
        }
    }
}
