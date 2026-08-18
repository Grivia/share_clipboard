using System.Text.Json;

namespace FastCopy.Windows;

internal sealed class LocalStore
{
    private readonly object _gate = new();
    private readonly string _statePath;
    private readonly DpapiSecretStore _secretStore;

    public LocalStore(string? directory = null)
    {
        var storageDirectory = directory ?? StorageDirectory;
        Directory.CreateDirectory(storageDirectory);
        _statePath = Path.Combine(storageDirectory, "state.json");
        _secretStore = new DpapiSecretStore(Path.Combine(storageDirectory, "credentials.dat"));
    }

    public static string StorageDirectory => Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
        "FastCopy");

    public StoredState LoadState()
    {
        lock (_gate)
        {
            try
            {
                if (!File.Exists(_statePath))
                {
                    return new StoredState();
                }
                return (JsonSerializer.Deserialize<StoredState>(File.ReadAllBytes(_statePath))
                    ?? new StoredState()).Normalize();
            }
            catch (Exception exception)
            {
                AppLog.Error("Could not read local state", exception);
                return new StoredState();
            }
        }
    }

    public void SaveState(StoredState state)
    {
        lock (_gate)
        {
            WriteAtomic(_statePath, JsonSerializer.SerializeToUtf8Bytes(state));
        }
    }

    public SecretState? LoadSecrets()
    {
        lock (_gate)
        {
            return _secretStore.Load();
        }
    }

    public void SaveSecrets(SecretState secrets)
    {
        lock (_gate)
        {
            _secretStore.Save(secrets);
        }
    }

    public void ClearSecrets()
    {
        lock (_gate)
        {
            _secretStore.Clear();
        }
    }

    internal static void WriteAtomic(string path, byte[] data)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        var temporaryPath = path + ".tmp." + Guid.NewGuid().ToString("N");
        try
        {
            File.WriteAllBytes(temporaryPath, data);
            File.Move(temporaryPath, path, true);
        }
        finally
        {
            try
            {
                File.Delete(temporaryPath);
            }
            catch (FileNotFoundException)
            {
            }
        }
    }
}
