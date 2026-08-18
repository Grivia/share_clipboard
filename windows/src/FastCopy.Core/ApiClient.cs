using System.Net;
using System.Net.Http.Json;
using System.Net.WebSockets;
using System.Text.Json;

namespace FastCopy.Core;

public sealed class FastCopyApiException : Exception
{
    public FastCopyApiException(HttpStatusCode statusCode, string code, string message)
        : base(message)
    {
        StatusCode = statusCode;
        Code = code;
    }

    public HttpStatusCode StatusCode { get; }
    public string Code { get; }
    public bool IsUnauthorized => StatusCode == HttpStatusCode.Unauthorized;
}

public sealed class FastCopyApiClient
{
    private const string UserAgent = "FastCopyWindows/0.1.1";
    private static readonly HttpClient Http = CreateHttpClient();
    private readonly Uri _baseUri;

    public FastCopyApiClient(string baseUrl)
    {
        var normalized = baseUrl.Trim().TrimEnd('/') + "/";
        if (!Uri.TryCreate(normalized, UriKind.Absolute, out var parsed)
            || (parsed.Scheme != Uri.UriSchemeHttps
                && !(parsed.Scheme == Uri.UriSchemeHttp && parsed.IsLoopback)))
        {
            throw new ArgumentException("服务端地址无效，非本机地址必须使用 HTTPS。", nameof(baseUrl));
        }
        _baseUri = parsed;
    }

    public Task<AuthResponse> AuthenticateAsync(AuthRequest request, CancellationToken cancellationToken) =>
        SendAsync<AuthResponse>(HttpMethod.Post, "v1/auth/session", null, request, cancellationToken);

    public Task<RefreshResponse> RefreshAsync(string refreshToken, CancellationToken cancellationToken) =>
        SendAsync<RefreshResponse>(
            HttpMethod.Post,
            "v1/auth/refresh",
            null,
            new RefreshRequest(refreshToken),
            cancellationToken);

    public Task LogoutAsync(string token, CancellationToken cancellationToken) =>
        SendWithoutResponseAsync(HttpMethod.Post, "v1/auth/logout", token, null, cancellationToken);

    public Task<ClipCreateResponse> UploadAsync(
        ClipUpload upload,
        string token,
        CancellationToken cancellationToken) =>
        SendAsync<ClipCreateResponse>(HttpMethod.Post, "v1/clips", token, upload, cancellationToken);

    public Task<ClipsResponse> ClipsAsync(
        long afterSeq,
        string token,
        CancellationToken cancellationToken) =>
        SendAsync<ClipsResponse>(
            HttpMethod.Get,
            $"v1/clips?after_seq={afterSeq}&limit=200",
            token,
            null,
            cancellationToken);

    public Task AcknowledgeAsync(long seq, string token, CancellationToken cancellationToken) =>
        SendWithoutResponseAsync(
            HttpMethod.Post,
            "v1/sync/ack",
            token,
            new AcknowledgeRequest(seq),
            cancellationToken);

    public Task<DevicesResponse> DevicesAsync(string token, CancellationToken cancellationToken) =>
        SendAsync<DevicesResponse>(HttpMethod.Get, "v1/devices", token, null, cancellationToken);

    public Task RevokeDeviceAsync(string deviceId, string token, CancellationToken cancellationToken) =>
        SendWithoutResponseAsync(
            HttpMethod.Post,
            $"v1/devices/{Uri.EscapeDataString(deviceId)}/revoke",
            token,
            null,
            cancellationToken);

    public async Task<ClientWebSocket> ConnectWebSocketAsync(
        string token,
        CancellationToken cancellationToken)
    {
        var socket = new ClientWebSocket();
        socket.Options.SetRequestHeader("Authorization", $"Bearer {token}");
        socket.Options.SetRequestHeader("User-Agent", UserAgent);
        socket.Options.KeepAliveInterval = TimeSpan.Zero;
        try
        {
            await socket.ConnectAsync(WebSocketUri(), cancellationToken).ConfigureAwait(false);
            return socket;
        }
        catch
        {
            socket.Dispose();
            throw;
        }
    }

    private async Task<T> SendAsync<T>(
        HttpMethod method,
        string path,
        string? token,
        object? body,
        CancellationToken cancellationToken)
    {
        using var response = await SendAsync(method, path, token, body, cancellationToken)
            .ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
        return await response.Content.ReadFromJsonAsync<T>(FastCopyJson.Options, cancellationToken)
            .ConfigureAwait(false)
            ?? throw new InvalidDataException("服务端返回了空响应。");
    }

    private async Task SendWithoutResponseAsync(
        HttpMethod method,
        string path,
        string? token,
        object? body,
        CancellationToken cancellationToken)
    {
        using var response = await SendAsync(method, path, token, body, cancellationToken)
            .ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
    }

    private async Task<HttpResponseMessage> SendAsync(
        HttpMethod method,
        string path,
        string? token,
        object? body,
        CancellationToken cancellationToken)
    {
        using var request = new HttpRequestMessage(method, new Uri(_baseUri, path));
        request.Headers.Accept.ParseAdd("application/json");
        request.Headers.UserAgent.ParseAdd(UserAgent);
        if (!string.IsNullOrEmpty(token))
        {
            request.Headers.Authorization = new("Bearer", token);
        }
        if (body is not null)
        {
            request.Content = JsonContent.Create(body, options: FastCopyJson.Options);
        }
        return await Http.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken)
            .ConfigureAwait(false);
    }

    private static async Task EnsureSuccessAsync(
        HttpResponseMessage response,
        CancellationToken cancellationToken)
    {
        if (response.IsSuccessStatusCode)
        {
            return;
        }
        ApiErrorEnvelope? envelope = null;
        try
        {
            envelope = await response.Content.ReadFromJsonAsync<ApiErrorEnvelope>(
                FastCopyJson.Options,
                cancellationToken).ConfigureAwait(false);
        }
        catch (JsonException)
        {
        }
        throw new FastCopyApiException(
            response.StatusCode,
            envelope?.Error.Code ?? $"HTTP_{(int)response.StatusCode}",
            envelope?.Error.Message ?? $"服务端请求失败（HTTP {(int)response.StatusCode}）。");
    }

    private Uri WebSocketUri()
    {
        var builder = new UriBuilder(new Uri(_baseUri, "v1/events/ws"))
        {
            Scheme = _baseUri.Scheme == Uri.UriSchemeHttps ? "wss" : "ws"
        };
        return builder.Uri;
    }

    private static HttpClient CreateHttpClient()
    {
        var handler = new SocketsHttpHandler
        {
            AutomaticDecompression = DecompressionMethods.GZip | DecompressionMethods.Deflate,
            ConnectTimeout = TimeSpan.FromSeconds(10),
            PooledConnectionIdleTimeout = TimeSpan.FromMinutes(2),
            PooledConnectionLifetime = TimeSpan.FromMinutes(10),
            MaxConnectionsPerServer = 4
        };
        return new HttpClient(handler) { Timeout = TimeSpan.FromSeconds(30) };
    }
}
