using System.Text.Json;

namespace FastCopy.Core;

public static class FastCopyJson
{
    public static readonly JsonSerializerOptions Options = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
        WriteIndented = false
    };
}

public sealed record DeviceInput(
    string InstallationId,
    string ReportedName,
    string Platform,
    string OsVersion,
    string AppVersion);

public sealed record AuthRequest(string Account, string Password, DeviceInput Device);

public sealed record UserModel(string Id, string Account, string CreatedAt);

public sealed record DeviceModel(
    string Id,
    string? InstallationId,
    string ReportedName,
    string? CustomName,
    string DisplayName,
    string Platform,
    string OsVersion,
    string AppVersion,
    string FirstLoginAt,
    string LastLoginAt,
    string? LastSeenAt,
    string? RevokedAt,
    bool LoggedIn,
    bool Online,
    bool Current);

public sealed record SessionTokens(
    string AccessToken,
    string AccessExpiresAt,
    string RefreshToken,
    string RefreshExpiresAt);

public sealed record AuthResponse(UserModel User, DeviceModel Device, SessionTokens Tokens);

public sealed record RefreshResponse(SessionTokens Tokens);

public sealed record DevicesResponse(IReadOnlyList<DeviceModel> Devices);

public sealed record ClipUpload(
    string ClientEventId,
    string ContentType,
    string Algorithm,
    string Nonce,
    string Ciphertext);

public sealed record ClipEvent(
    string EventId,
    string ClientEventId,
    long Seq,
    string OriginDeviceId,
    string OriginName,
    string ContentType,
    string Algorithm,
    string Nonce,
    string Ciphertext,
    string CreatedAt,
    string ExpiresAt);

public sealed record ClipCreateResponse(ClipEvent Event, string Status);

public sealed record ClipsResponse(IReadOnlyList<ClipEvent> Clips);

public sealed record WebSocketEnvelope(string Type);

internal sealed record RefreshRequest(string RefreshToken);

internal sealed record AcknowledgeRequest(long Seq);

internal sealed record ApiErrorDetail(string Code, string Message);

internal sealed record ApiErrorEnvelope(ApiErrorDetail Error);
