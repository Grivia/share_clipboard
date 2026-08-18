using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace FastCopy.Windows;

internal sealed class DpapiSecretStore
{
    private const int CryptProtectUiForbidden = 0x1;
    private static readonly byte[] Entropy = Encoding.UTF8.GetBytes("FastCopy.Windows.Secrets.v1");
    private readonly string _path;

    public DpapiSecretStore(string path)
    {
        _path = path;
    }

    public SecretState? Load()
    {
        if (!File.Exists(_path))
        {
            return null;
        }

        byte[]? plaintext = null;
        try
        {
            plaintext = Unprotect(File.ReadAllBytes(_path));
            return JsonSerializer.Deserialize<SecretState>(plaintext);
        }
        catch (Exception exception)
        {
            AppLog.Error("Could not read protected credentials", exception);
            return null;
        }
        finally
        {
            if (plaintext is not null)
            {
                CryptographicOperations.ZeroMemory(plaintext);
            }
        }
    }

    public void Save(SecretState state)
    {
        var plaintext = JsonSerializer.SerializeToUtf8Bytes(state);
        try
        {
            var protectedData = Protect(plaintext);
            try
            {
                LocalStore.WriteAtomic(_path, protectedData);
            }
            finally
            {
                CryptographicOperations.ZeroMemory(protectedData);
            }
        }
        finally
        {
            CryptographicOperations.ZeroMemory(plaintext);
        }
    }

    public void Clear()
    {
        try
        {
            File.Delete(_path);
        }
        catch (FileNotFoundException)
        {
        }
    }

    private static byte[] Protect(byte[] data) => Transform(data, protect: true);

    private static byte[] Unprotect(byte[] data) => Transform(data, protect: false);

    private static byte[] Transform(byte[] data, bool protect)
    {
        var input = DataBlob.FromBytes(data);
        var entropy = DataBlob.FromBytes(Entropy);
        var output = new DataBlob();
        IntPtr description = IntPtr.Zero;
        try
        {
            var succeeded = protect
                ? CryptProtectData(
                    ref input,
                    "FastCopy credentials",
                    ref entropy,
                    IntPtr.Zero,
                    IntPtr.Zero,
                    CryptProtectUiForbidden,
                    out output)
                : CryptUnprotectData(
                    ref input,
                    out description,
                    ref entropy,
                    IntPtr.Zero,
                    IntPtr.Zero,
                    CryptProtectUiForbidden,
                    out output);
            if (!succeeded)
            {
                throw new Win32Exception(Marshal.GetLastWin32Error());
            }
            return output.ToBytes();
        }
        finally
        {
            input.Dispose();
            entropy.Dispose();
            output.DisposeLocal();
            if (description != IntPtr.Zero)
            {
                LocalFree(description);
            }
        }
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct DataBlob
    {
        public int Size;
        public IntPtr Data;

        public static DataBlob FromBytes(byte[] bytes)
        {
            var blob = new DataBlob
            {
                Size = bytes.Length,
                Data = Marshal.AllocHGlobal(Math.Max(bytes.Length, 1))
            };
            if (bytes.Length > 0)
            {
                Marshal.Copy(bytes, 0, blob.Data, bytes.Length);
            }
            return blob;
        }

        public readonly byte[] ToBytes()
        {
            var result = new byte[Size];
            if (Size > 0)
            {
                Marshal.Copy(Data, result, 0, Size);
            }
            return result;
        }

        public void Dispose()
        {
            if (Data == IntPtr.Zero)
            {
                return;
            }
            ZeroUnmanaged(Data, Size);
            Marshal.FreeHGlobal(Data);
            Data = IntPtr.Zero;
            Size = 0;
        }

        public void DisposeLocal()
        {
            if (Data == IntPtr.Zero)
            {
                return;
            }
            ZeroUnmanaged(Data, Size);
            LocalFree(Data);
            Data = IntPtr.Zero;
            Size = 0;
        }
    }

    private static void ZeroUnmanaged(IntPtr pointer, int length)
    {
        if (pointer == IntPtr.Zero || length <= 0)
        {
            return;
        }
        Marshal.Copy(new byte[length], 0, pointer, length);
    }

    [DllImport("crypt32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CryptProtectData(
        ref DataBlob dataIn,
        string? description,
        ref DataBlob optionalEntropy,
        IntPtr reserved,
        IntPtr prompt,
        int flags,
        out DataBlob dataOut);

    [DllImport("crypt32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CryptUnprotectData(
        ref DataBlob dataIn,
        out IntPtr description,
        ref DataBlob optionalEntropy,
        IntPtr reserved,
        IntPtr prompt,
        int flags,
        out DataBlob dataOut);

    [DllImport("kernel32.dll")]
    private static extern IntPtr LocalFree(IntPtr memory);
}
