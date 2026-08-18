using FastCopy.Core;

namespace FastCopy.Windows;

internal sealed class StoredState
{
    public string ServerUrl { get; set; } = "https://zhy.hair/fastcopy";
    public string Account { get; set; } = "";
    public bool SyncEnabled { get; set; } = true;
    public string InstallationId { get; set; } = Guid.NewGuid().ToString("D");
    public string? UserId { get; set; }
    public string? DeviceId { get; set; }
    public string? PendingOwner { get; set; }
    public Dictionary<string, long> LastSequenceByUser { get; set; } = new();
    public List<ClipUpload> PendingUploads { get; set; } = new();

    public StoredState Normalize()
    {
        ServerUrl = string.IsNullOrWhiteSpace(ServerUrl)
            ? "https://zhy.hair/fastcopy"
            : ServerUrl.Trim().TrimEnd('/');
        Account ??= "";
        if (!Guid.TryParse(InstallationId, out _))
        {
            InstallationId = Guid.NewGuid().ToString("D");
        }
        LastSequenceByUser ??= new();
        PendingUploads ??= new();
        PendingUploads = PendingUploads.TakeLast(100).ToList();
        return this;
    }

    public long LastSequence
    {
        get => UserId is not null && LastSequenceByUser.TryGetValue(UserId, out var value) ? value : 0;
        set
        {
            if (UserId is not null)
            {
                LastSequenceByUser[UserId] = value;
            }
        }
    }

    public StoredState Copy() => new()
    {
        ServerUrl = ServerUrl,
        Account = Account,
        SyncEnabled = SyncEnabled,
        InstallationId = InstallationId,
        UserId = UserId,
        DeviceId = DeviceId,
        PendingOwner = PendingOwner,
        LastSequenceByUser = new Dictionary<string, long>(LastSequenceByUser),
        PendingUploads = new List<ClipUpload>(PendingUploads)
    };
}

internal sealed record SecretState(
    string AccessToken,
    string RefreshToken,
    string SharedKeyBase64,
    int KeyDerivationVersion);
