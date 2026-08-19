using System.Net.WebSockets;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using FastCopy.Core;

namespace FastCopy.Windows;

internal sealed record ClientSnapshot(
    bool IsAuthenticated,
    bool IsConnected,
    bool IsBusy,
    bool SyncEnabled,
    string Status,
    string? Error,
    string Account,
    string ServerUrl,
    int PendingCount,
    IReadOnlyList<DeviceModel> Devices);

internal sealed class SyncEngine : IAsyncDisposable
{
    private const int MaxPlaintextBytes = 256 * 1024 - 16;
    private const string AppVersion = "0.1.2";
    private readonly object _gate = new();
    private readonly object _persistenceGate = new();
    private readonly LocalStore _store;
    private readonly ClipboardWatcher _clipboard;
    private readonly SynchronizationContext _uiContext;
    private readonly int _uiThreadId;
    private readonly SemaphoreSlim _flushGate = new(1, 1);
    private readonly SemaphoreSlim _syncGate = new(1, 1);
    private readonly SemaphoreSlim _refreshGate = new(1, 1);
    private readonly SemaphoreSlim _devicesGate = new(1, 1);
    private readonly SemaphoreSlim _pendingWake = new(0);
    private readonly SemaphoreSlim _reconciliationWake = new(0);
    private StoredState _state;
    private SecretState? _secrets;
    private byte[]? _sharedKey;
    private IReadOnlyList<DeviceModel> _devices = Array.Empty<DeviceModel>();
    private CancellationTokenSource? _servicesCancellation;
    private Task? _webSocketTask;
    private Task? _pendingTask;
    private Task? _reconciliationTask;
    private bool _started;
    private bool _connected;
    private bool _busy;
    private bool _deviceViewActive;
    private bool _clearingAuthentication;
    private bool _disposed;
    private string _status;
    private string? _error;
    private int _syncRequested;

    public SyncEngine(LocalStore store, ClipboardWatcher clipboard)
    {
        _store = store;
        _clipboard = clipboard;
        _uiContext = SynchronizationContext.Current
            ?? throw new InvalidOperationException("SyncEngine must be created on the UI thread.");
        _uiThreadId = Environment.CurrentManagedThreadId;
        _state = store.LoadState();
        _secrets = store.LoadSecrets();

        if (!TryRestoreAuthentication())
        {
            ClearInvalidStoredAuthentication();
        }
        _status = IsAuthenticated
            ? (_state.SyncEnabled ? "正在连接" : "同步已暂停")
            : "尚未登录";
        _clipboard.TextChanged += ClipboardTextChanged;
    }

    public event EventHandler<ClientSnapshot>? SnapshotChanged;

    public ClientSnapshot Snapshot
    {
        get
        {
            lock (_gate)
            {
                return CreateSnapshotLocked();
            }
        }
    }

    private bool IsAuthenticated =>
        _secrets is not null
        && _sharedKey is not null
        && _state.UserId is not null
        && _state.DeviceId is not null;

    public async Task StartAsync()
    {
        lock (_gate)
        {
            if (_started)
            {
                return;
            }
            _started = true;
        }

        if (IsAuthenticated && _state.SyncEnabled)
        {
            await StartSyncServicesAsync().ConfigureAwait(false);
        }
        Publish();
    }

    public async Task AuthenticateAsync(string serverUrl, string account, string password)
    {
        lock (_gate)
        {
            if (_busy)
            {
                return;
            }
            _busy = true;
            _error = null;
            _status = "正在登录";
        }
        Publish();

        byte[]? derivedKey = null;
        try
        {
            var normalizedServer = NormalizeServerUrl(serverUrl);
            var normalizedAccount = account.Trim();
            ValidateCredentials(normalizedAccount, password);
            var client = new FastCopyApiClient(normalizedServer);
            var response = await client.AuthenticateAsync(
                new AuthRequest(normalizedAccount, password, DeviceInput()),
                CancellationToken.None).ConfigureAwait(false);
            derivedKey = await Task.Run(
                () => FastCopyCrypto.DeriveKey(response.User.Account, password)).ConfigureAwait(false);

            await StopSyncServicesAsync().ConfigureAwait(false);
            AcceptAuthentication(normalizedServer, response, derivedKey);
            derivedKey = null;
            await StartSyncServicesAsync().ConfigureAwait(false);
            await RefreshDevicesAsync().ConfigureAwait(false);
        }
        catch (Exception exception)
        {
            AppLog.Error("Authentication failed", exception);
            lock (_gate)
            {
                _error = UserMessage(exception);
                _status = "登录失败";
            }
        }
        finally
        {
            if (derivedKey is not null)
            {
                CryptographicOperations.ZeroMemory(derivedKey);
            }
            lock (_gate)
            {
                _busy = false;
            }
            Publish();
        }
    }

    public async Task LogoutAsync()
    {
        string? token;
        string serverUrl;
        lock (_gate)
        {
            token = _secrets?.AccessToken;
            serverUrl = _state.ServerUrl;
            _busy = true;
            _error = null;
        }
        Publish();

        if (token is not null)
        {
            try
            {
                using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(5));
                await new FastCopyApiClient(serverUrl).LogoutAsync(token, timeout.Token)
                    .ConfigureAwait(false);
            }
            catch (Exception exception)
            {
                AppLog.Error("Remote logout failed", exception);
            }
        }

        await ClearAuthenticationAsync("尚未登录").ConfigureAwait(false);
        lock (_gate)
        {
            _busy = false;
        }
        Publish();
    }

    public async Task SetSyncEnabledAsync(bool enabled)
    {
        bool shouldStart;
        lock (_gate)
        {
            _state.SyncEnabled = enabled;
            _error = null;
            _status = enabled ? "正在连接" : "同步已暂停";
            shouldStart = enabled && IsAuthenticated;
        }
        PersistState();

        if (shouldStart)
        {
            await StartSyncServicesAsync().ConfigureAwait(false);
        }
        else
        {
            await StopSyncServicesAsync().ConfigureAwait(false);
        }
        Publish();
    }

    public async Task RefreshNowAsync()
    {
        lock (_gate)
        {
            _error = null;
            if (IsAuthenticated && _state.SyncEnabled)
            {
                _status = "正在同步";
            }
        }
        Publish();

        CancellationToken token;
        lock (_gate)
        {
            token = _servicesCancellation?.Token ?? CancellationToken.None;
        }
        await FlushPendingUploadsAsync(token).ConfigureAwait(false);
        await SynchronizeAsync(token).ConfigureAwait(false);
        await RefreshDevicesAsync(token).ConfigureAwait(false);
        WakeReconciliation();
    }

    public void SetDeviceViewActive(bool active)
    {
        lock (_gate)
        {
            _deviceViewActive = active;
        }
        if (active)
        {
            _ = RefreshDevicesAsync();
        }
    }

    public async Task RefreshDevicesAsync(CancellationToken cancellationToken = default)
    {
        if (!IsAuthenticated)
        {
            return;
        }
        await _devicesGate.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            var response = await AuthorizedAsync(
                (client, token, ct) => client.DevicesAsync(token, ct),
                cancellationToken).ConfigureAwait(false);
            lock (_gate)
            {
                _devices = response.Devices
                    .OrderByDescending(device => device.Current)
                    .ThenByDescending(device => device.Online)
                    .ThenByDescending(device => device.LastLoginAt, StringComparer.Ordinal)
                    .ToArray();
            }
            Publish();
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
        }
        catch (Exception exception)
        {
            SetError(exception);
        }
        finally
        {
            _devicesGate.Release();
        }
    }

    public async Task RevokeDeviceAsync(DeviceModel device)
    {
        if (!device.CanRevoke)
        {
            return;
        }
        try
        {
            await AuthorizedAsync<object?>(async (client, token, cancellationToken) =>
            {
                await client.RevokeDeviceAsync(device.Id, token, cancellationToken)
                    .ConfigureAwait(false);
                return null;
            }, CancellationToken.None).ConfigureAwait(false);
            await RefreshDevicesAsync().ConfigureAwait(false);
        }
        catch (Exception exception)
        {
            SetError(exception);
        }
    }

    public async Task SetDeviceRoleAsync(DeviceModel device, string role)
    {
        if (!device.CanChangeRole || (role != "admin" && role != "member"))
        {
            return;
        }
        try
        {
            await AuthorizedAsync<object?>(async (client, token, cancellationToken) =>
            {
                await client.UpdateDeviceRoleAsync(device.Id, role, token, cancellationToken)
                    .ConfigureAwait(false);
                return null;
            }, CancellationToken.None).ConfigureAwait(false);
            await RefreshDevicesAsync().ConfigureAwait(false);
        }
        catch (Exception exception)
        {
            SetError(exception);
        }
    }

    public async ValueTask DisposeAsync()
    {
        Task[] runningTasks;
        lock (_gate)
        {
            if (_disposed)
            {
                return;
            }
            _disposed = true;
            runningTasks = new[] { _webSocketTask, _pendingTask, _reconciliationTask }
                .OfType<Task>()
                .ToArray();
        }
        _clipboard.TextChanged -= ClipboardTextChanged;
        await StopSyncServicesAsync().ConfigureAwait(false);
        try
        {
            await Task.WhenAll(runningTasks).ConfigureAwait(false);
        }
        catch (OperationCanceledException)
        {
        }
        catch (Exception exception)
        {
            AppLog.Error("A background service did not stop cleanly", exception);
        }
        _clipboard.Dispose();
        lock (_gate)
        {
            if (_sharedKey is not null)
            {
                CryptographicOperations.ZeroMemory(_sharedKey);
                _sharedKey = null;
            }
        }
        _flushGate.Dispose();
        _syncGate.Dispose();
        _refreshGate.Dispose();
        _devicesGate.Dispose();
        _pendingWake.Dispose();
        _reconciliationWake.Dispose();
    }

    private bool TryRestoreAuthentication()
    {
        if (_secrets is null
            || _secrets.KeyDerivationVersion != FastCopyCrypto.KeyDerivationVersion
            || string.IsNullOrWhiteSpace(_secrets.AccessToken)
            || string.IsNullOrWhiteSpace(_secrets.RefreshToken)
            || _state.UserId is null
            || _state.DeviceId is null)
        {
            return false;
        }
        try
        {
            var key = Convert.FromBase64String(_secrets.SharedKeyBase64);
            if (key.Length != 32)
            {
                CryptographicOperations.ZeroMemory(key);
                return false;
            }
            _sharedKey = key;
            if (_state.PendingOwner is not null && _state.PendingOwner != _state.UserId)
            {
                _state.PendingUploads.Clear();
            }
            _state.PendingOwner = _state.UserId;
            PersistState();
            return true;
        }
        catch (FormatException)
        {
            return false;
        }
    }

    private void ClearInvalidStoredAuthentication()
    {
        _secrets = null;
        _state.UserId = null;
        _state.DeviceId = null;
        _state.PendingOwner = null;
        _state.PendingUploads.Clear();
        _store.ClearSecrets();
        PersistState();
    }

    private void AcceptAuthentication(string serverUrl, AuthResponse response, byte[] derivedKey)
    {
        var keyBase64 = Convert.ToBase64String(derivedKey);
        var secrets = new SecretState(
            response.Tokens.AccessToken,
            response.Tokens.RefreshToken,
            keyBase64,
            FastCopyCrypto.KeyDerivationVersion);
        _store.SaveSecrets(secrets);

        lock (_gate)
        {
            if (_sharedKey is not null)
            {
                CryptographicOperations.ZeroMemory(_sharedKey);
            }
            if (_state.PendingOwner is not null && _state.PendingOwner != response.User.Id)
            {
                _state.PendingUploads.Clear();
            }
            _state.ServerUrl = serverUrl;
            _state.Account = response.User.Account;
            _state.UserId = response.User.Id;
            _state.DeviceId = response.Device.Id;
            _state.PendingOwner = response.User.Id;
            _secrets = secrets;
            _sharedKey = derivedKey;
            _devices = new[] { response.Device };
            _connected = false;
            _error = null;
            _status = _state.SyncEnabled ? "正在连接" : "同步已暂停";
        }
        PersistState();
        Publish();
    }

    private async Task StartSyncServicesAsync()
    {
        CancellationTokenSource services;
        lock (_gate)
        {
            if (!IsAuthenticated || !_state.SyncEnabled || _servicesCancellation is not null)
            {
                return;
            }
            services = new CancellationTokenSource();
            _servicesCancellation = services;
            _status = "正在连接";
        }

        try
        {
            await OnUiAsync(_clipboard.Start).ConfigureAwait(false);
        }
        catch
        {
            lock (_gate)
            {
                if (ReferenceEquals(_servicesCancellation, services))
                {
                    _servicesCancellation = null;
                }
            }
            services.Dispose();
            throw;
        }

        bool staleStart;
        bool stopStaleListener;
        lock (_gate)
        {
            staleStart = !ReferenceEquals(_servicesCancellation, services)
                || services.IsCancellationRequested;
            stopStaleListener = staleStart && _servicesCancellation is null;
        }
        if (staleStart)
        {
            if (stopStaleListener)
            {
                await OnUiAsync(_clipboard.Stop).ConfigureAwait(false);
            }
            return;
        }

        _webSocketTask = Task.Run(() => WebSocketLoopAsync(services.Token));
        _pendingTask = Task.Run(() => PendingUploadLoopAsync(services.Token));
        _reconciliationTask = Task.Run(() => ReconciliationLoopAsync(services.Token));
        WakePending();
        WakeReconciliation();
        await SynchronizeAsync(services.Token).ConfigureAwait(false);
    }

    private async Task StopSyncServicesAsync()
    {
        CancellationTokenSource? services;
        lock (_gate)
        {
            services = _servicesCancellation;
            _servicesCancellation = null;
            _connected = false;
        }
        services?.Cancel();
        try
        {
            await OnUiAsync(_clipboard.Stop).ConfigureAwait(false);
        }
        catch (Exception exception)
        {
            AppLog.Error("Could not stop clipboard listener", exception);
        }
        services?.Dispose();
        Publish();
    }

    private void ClipboardTextChanged(object? sender, string text)
    {
        _ = HandleLocalClipboardAsync(text);
    }

    private async Task HandleLocalClipboardAsync(string text)
    {
        byte[]? key = null;
        try
        {
            var textLength = Encoding.UTF8.GetByteCount(text);
            if (textLength > MaxPlaintextBytes)
            {
                lock (_gate)
                {
                    _error = "文本超过 256 KiB，未同步";
                }
                Publish();
                return;
            }

            lock (_gate)
            {
                if (!IsAuthenticated || !_state.SyncEnabled || _sharedKey is null)
                {
                    return;
                }
                key = (byte[])_sharedKey.Clone();
            }
            var upload = FastCopyCrypto.Encrypt(text, key, Guid.NewGuid().ToString("D"));
            lock (_gate)
            {
                _state.PendingUploads.Add(upload);
                if (_state.PendingUploads.Count > 100)
                {
                    _state.PendingUploads.RemoveRange(0, _state.PendingUploads.Count - 100);
                }
                _status = "正在发送";
                _error = null;
            }
            PersistState();
            Publish();
            WakePending();
        }
        catch (Exception exception)
        {
            SetError(exception);
        }
        finally
        {
            if (key is not null)
            {
                CryptographicOperations.ZeroMemory(key);
            }
        }
        await Task.CompletedTask;
    }

    private async Task PendingUploadLoopAsync(CancellationToken cancellationToken)
    {
        var retryAttempt = 0;
        try
        {
            while (true)
            {
                await _pendingWake.WaitAsync(cancellationToken).ConfigureAwait(false);
                while (HasPendingUploads() && !cancellationToken.IsCancellationRequested)
                {
                    if (await FlushPendingUploadsAsync(cancellationToken).ConfigureAwait(false))
                    {
                        retryAttempt = 0;
                        break;
                    }
                    await Task.Delay(SyncTiming.PendingRetry(retryAttempt), cancellationToken)
                        .ConfigureAwait(false);
                    retryAttempt++;
                }
            }
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
        }
    }

    private async Task<bool> FlushPendingUploadsAsync(CancellationToken cancellationToken)
    {
        if (!IsAuthenticated || !_state.SyncEnabled)
        {
            return true;
        }
        await _flushGate.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            while (true)
            {
                ClipUpload? upload;
                lock (_gate)
                {
                    upload = _state.PendingUploads.FirstOrDefault();
                }
                if (upload is null)
                {
                    return true;
                }

                try
                {
                    _ = await AuthorizedAsync(
                        (client, token, ct) => client.UploadAsync(upload, token, ct),
                        cancellationToken).ConfigureAwait(false);
                    lock (_gate)
                    {
                        var index = _state.PendingUploads.FindIndex(
                            item => item.ClientEventId == upload.ClientEventId);
                        if (index >= 0)
                        {
                            _state.PendingUploads.RemoveAt(index);
                        }
                        _status = "剪贴板已同步";
                        _error = null;
                    }
                    PersistState();
                    Publish();
                }
                catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
                {
                    return false;
                }
                catch (Exception exception)
                {
                    AppLog.Error("Pending clipboard upload failed", exception);
                    lock (_gate)
                    {
                        _status = "等待网络恢复";
                        _error = UserMessage(exception);
                    }
                    Publish();
                    return false;
                }
            }
        }
        finally
        {
            _flushGate.Release();
        }
    }

    private Task SynchronizeAsync(CancellationToken cancellationToken)
    {
        Interlocked.Exchange(ref _syncRequested, 1);
        return RunSynchronizationAsync(cancellationToken);
    }

    private async Task RunSynchronizationAsync(CancellationToken cancellationToken)
    {
        while (true)
        {
            if (!await _syncGate.WaitAsync(0, cancellationToken).ConfigureAwait(false))
            {
                return;
            }
            try
            {
                while (Interlocked.Exchange(ref _syncRequested, 0) == 1)
                {
                    await SynchronizeOnceAsync(cancellationToken).ConfigureAwait(false);
                }
            }
            finally
            {
                _syncGate.Release();
            }
            if (Volatile.Read(ref _syncRequested) == 0)
            {
                return;
            }
        }
    }

    private async Task SynchronizeOnceAsync(CancellationToken cancellationToken)
    {
        long initialSequence;
        string? ownDeviceId;
        byte[]? key;
        lock (_gate)
        {
            if (!IsAuthenticated || !_state.SyncEnabled || _sharedKey is null)
            {
                return;
            }
            initialSequence = _state.LastSequence;
            ownDeviceId = _state.DeviceId;
            key = (byte[])_sharedKey.Clone();
        }

        var cursor = initialSequence;
        try
        {
            while (true)
            {
                var response = await AuthorizedAsync(
                    (client, token, ct) => client.ClipsAsync(cursor, token, ct),
                    cancellationToken).ConfigureAwait(false);
                foreach (var clip in response.Clips)
                {
                    cursor = Math.Max(cursor, clip.Seq);
                    if (clip.OriginDeviceId == ownDeviceId)
                    {
                        continue;
                    }
                    try
                    {
                        var text = FastCopyCrypto.Decrypt(clip, key);
                        await _clipboard.WriteWithoutUploadingAsync(text, cancellationToken)
                            .ConfigureAwait(false);
                        lock (_gate)
                        {
                            _status = $"已接收来自 {clip.OriginName} 的文本";
                            _error = null;
                        }
                        Publish();
                    }
                    catch (CryptographicException exception)
                    {
                        AppLog.Error("Could not decrypt a clipboard event", exception);
                        lock (_gate)
                        {
                            _error = $"无法解密来自 {clip.OriginName} 的文本，请重新登录";
                        }
                        Publish();
                    }
                }
                if (response.Clips.Count < 200)
                {
                    break;
                }
            }

            if (cursor > initialSequence)
            {
                lock (_gate)
                {
                    _state.LastSequence = cursor;
                }
                PersistState();
                await AuthorizedAsync<object?>(async (client, token, ct) =>
                {
                    await client.AcknowledgeAsync(cursor, token, ct).ConfigureAwait(false);
                    return null;
                }, cancellationToken).ConfigureAwait(false);
            }
            lock (_gate)
            {
                if (_status is "正在连接" or "正在同步" or "等待网络恢复")
                {
                    _status = "同步就绪";
                    _error = null;
                }
            }
            Publish();
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
        }
        catch (Exception exception)
        {
            AppLog.Error("Clipboard reconciliation failed", exception);
            lock (_gate)
            {
                _status = "等待网络恢复";
                _error = UserMessage(exception);
            }
            Publish();
        }
        finally
        {
            CryptographicOperations.ZeroMemory(key);
        }
    }

    private async Task ReconciliationLoopAsync(CancellationToken cancellationToken)
    {
        try
        {
            while (true)
            {
                TimeSpan delay;
                lock (_gate)
                {
                    delay = _connected
                        ? SyncTiming.ConnectedReconciliation
                        : SyncTiming.DisconnectedReconciliation;
                }

                using var iteration = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
                var delayTask = Task.Delay(delay, iteration.Token);
                var wakeTask = _reconciliationWake.WaitAsync(iteration.Token);
                var completed = await Task.WhenAny(delayTask, wakeTask).ConfigureAwait(false);
                iteration.Cancel();
                try
                {
                    await completed.ConfigureAwait(false);
                }
                catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
                {
                    throw;
                }
                catch (OperationCanceledException)
                {
                }
                if (completed == wakeTask)
                {
                    continue;
                }

                await FlushPendingUploadsAsync(cancellationToken).ConfigureAwait(false);
                await SynchronizeAsync(cancellationToken).ConfigureAwait(false);
            }
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
        }
    }

    private async Task WebSocketLoopAsync(CancellationToken cancellationToken)
    {
        var retryDelay = TimeSpan.FromSeconds(1);
        try
        {
            while (!cancellationToken.IsCancellationRequested)
            {
                ClientWebSocket? socket = null;
                try
                {
                    string token;
                    string serverUrl;
                    lock (_gate)
                    {
                        if (!IsAuthenticated || !_state.SyncEnabled || _secrets is null)
                        {
                            return;
                        }
                        token = _secrets.AccessToken;
                        serverUrl = _state.ServerUrl;
                    }
                    socket = await new FastCopyApiClient(serverUrl)
                        .ConnectWebSocketAsync(token, cancellationToken).ConfigureAwait(false);
                    retryDelay = TimeSpan.FromSeconds(1);

                    while (socket.State == WebSocketState.Open && !cancellationToken.IsCancellationRequested)
                    {
                        var payload = await ReceiveMessageAsync(socket, cancellationToken)
                            .ConfigureAwait(false);
                        if (payload is null)
                        {
                            break;
                        }
                        SetConnected(true);
                        WebSocketEnvelope? envelope;
                        try
                        {
                            envelope = JsonSerializer.Deserialize<WebSocketEnvelope>(
                                payload,
                                FastCopyJson.Options);
                        }
                        catch (JsonException)
                        {
                            continue;
                        }
                        if (envelope is null)
                        {
                            continue;
                        }
                        switch (envelope.Type)
                        {
                            case "hello":
                                lock (_gate)
                                {
                                    _status = "同步就绪";
                                    _error = null;
                                }
                                Publish();
                                WakePending();
                                await SynchronizeAsync(cancellationToken).ConfigureAwait(false);
                                break;
                            case "clip.created":
                                await SynchronizeAsync(cancellationToken).ConfigureAwait(false);
                                break;
                            case "device.logged_in":
                            case "device.updated":
                            case "device.revoked":
                            case "device.presence_changed":
                                bool refreshDevices;
                                lock (_gate)
                                {
                                    refreshDevices = _deviceViewActive;
                                }
                                if (refreshDevices)
                                {
                                    await RefreshDevicesAsync(cancellationToken).ConfigureAwait(false);
                                }
                                break;
                        }
                    }
                }
                catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
                {
                    return;
                }
                catch (Exception exception)
                {
                    AppLog.Error("WebSocket connection failed", exception);
                }
                finally
                {
                    SetConnected(false);
                    socket?.Dispose();
                }

                lock (_gate)
                {
                    if (IsAuthenticated && _state.SyncEnabled)
                    {
                        _status = "正在重新连接";
                    }
                }
                Publish();
                await Task.Delay(retryDelay, cancellationToken).ConfigureAwait(false);
                retryDelay = TimeSpan.FromSeconds(Math.Min(retryDelay.TotalSeconds * 2, 30));
            }
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
        }
    }

    private async Task<T> AuthorizedAsync<T>(
        Func<FastCopyApiClient, string, CancellationToken, Task<T>> operation,
        CancellationToken cancellationToken)
    {
        string attemptedToken;
        string serverUrl;
        lock (_gate)
        {
            attemptedToken = _secrets?.AccessToken
                ?? throw new FastCopyApiException(
                    System.Net.HttpStatusCode.Unauthorized,
                    "NOT_AUTHENTICATED",
                    "请重新登录");
            serverUrl = _state.ServerUrl;
        }
        var client = new FastCopyApiClient(serverUrl);
        try
        {
            return await operation(client, attemptedToken, cancellationToken).ConfigureAwait(false);
        }
        catch (FastCopyApiException exception) when (exception.IsUnauthorized)
        {
            var renewedToken = await RefreshSessionAsync(client, attemptedToken, cancellationToken)
                .ConfigureAwait(false);
            return await operation(client, renewedToken, cancellationToken).ConfigureAwait(false);
        }
    }

    private async Task<string> RefreshSessionAsync(
        FastCopyApiClient client,
        string attemptedAccessToken,
        CancellationToken cancellationToken)
    {
        await _refreshGate.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            string refreshToken;
            lock (_gate)
            {
                if (_secrets is null)
                {
                    throw new FastCopyApiException(
                        System.Net.HttpStatusCode.Unauthorized,
                        "SESSION_EXPIRED",
                        "登录已过期");
                }
                if (_secrets.AccessToken != attemptedAccessToken)
                {
                    return _secrets.AccessToken;
                }
                refreshToken = _secrets.RefreshToken;
            }

            try
            {
                var response = await client.RefreshAsync(refreshToken, cancellationToken)
                    .ConfigureAwait(false);
                SecretState updated;
                lock (_gate)
                {
                    if (_secrets is null)
                    {
                        throw new InvalidOperationException("登录状态已失效。");
                    }
                    updated = _secrets with
                    {
                        AccessToken = response.Tokens.AccessToken,
                        RefreshToken = response.Tokens.RefreshToken
                    };
                    _secrets = updated;
                }
                _store.SaveSecrets(updated);
                return updated.AccessToken;
            }
            catch (FastCopyApiException exception) when (exception.IsUnauthorized)
            {
                await ClearAuthenticationAsync("登录已过期，请重新登录").ConfigureAwait(false);
                throw;
            }
        }
        finally
        {
            _refreshGate.Release();
        }
    }

    private async Task ClearAuthenticationAsync(string status)
    {
        lock (_gate)
        {
            if (_clearingAuthentication)
            {
                return;
            }
            _clearingAuthentication = true;
        }
        try
        {
            await StopSyncServicesAsync().ConfigureAwait(false);
            lock (_gate)
            {
                if (_sharedKey is not null)
                {
                    CryptographicOperations.ZeroMemory(_sharedKey);
                    _sharedKey = null;
                }
                _secrets = null;
                _state.UserId = null;
                _state.DeviceId = null;
                _state.PendingOwner = null;
                _state.PendingUploads.Clear();
                _devices = Array.Empty<DeviceModel>();
                _connected = false;
                _status = status;
                _error = null;
            }
            _store.ClearSecrets();
            PersistState();
        }
        finally
        {
            lock (_gate)
            {
                _clearingAuthentication = false;
            }
            Publish();
        }
    }

    private void SetConnected(bool connected)
    {
        lock (_gate)
        {
            if (_connected == connected)
            {
                return;
            }
            _connected = connected;
        }
        WakeReconciliation();
        Publish();
    }

    private void SetError(Exception exception)
    {
        AppLog.Error("FastCopy operation failed", exception);
        lock (_gate)
        {
            _error = UserMessage(exception);
        }
        Publish();
    }

    private void PersistState()
    {
        lock (_persistenceGate)
        {
            StoredState copy;
            lock (_gate)
            {
                copy = _state.Copy();
            }
            _store.SaveState(copy);
        }
    }

    private void Publish()
    {
        ClientSnapshot snapshot;
        lock (_gate)
        {
            snapshot = CreateSnapshotLocked();
        }
        _uiContext.Post(
            _ => SnapshotChanged?.Invoke(this, snapshot),
            null);
    }

    private ClientSnapshot CreateSnapshotLocked() => new(
        IsAuthenticated,
        _connected,
        _busy,
        _state.SyncEnabled,
        _status,
        _error,
        _state.Account,
        _state.ServerUrl,
        _state.PendingUploads.Count,
        _devices);

    private bool HasPendingUploads()
    {
        lock (_gate)
        {
            return IsAuthenticated && _state.SyncEnabled && _state.PendingUploads.Count > 0;
        }
    }

    private void WakePending()
    {
        if (_pendingWake.CurrentCount == 0)
        {
            _pendingWake.Release();
        }
    }

    private void WakeReconciliation()
    {
        if (_reconciliationWake.CurrentCount == 0)
        {
            _reconciliationWake.Release();
        }
    }

    private Task OnUiAsync(Action action)
    {
        if (Environment.CurrentManagedThreadId == _uiThreadId)
        {
            action();
            return Task.CompletedTask;
        }
        var completion = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        _uiContext.Post(_ =>
        {
            try
            {
                action();
                completion.TrySetResult();
            }
            catch (Exception exception)
            {
                completion.TrySetException(exception);
            }
        }, null);
        return completion.Task;
    }

    private DeviceInput DeviceInput()
    {
        var name = Environment.MachineName;
        if (name.Length > 64)
        {
            name = name[..64];
        }
        return new DeviceInput(
            _state.InstallationId,
            name,
            "windows",
            Environment.OSVersion.Version.ToString(),
            AppVersion);
    }

    private static async Task<byte[]?> ReceiveMessageAsync(
        ClientWebSocket socket,
        CancellationToken cancellationToken)
    {
        var buffer = new byte[4096];
        using var stream = new MemoryStream();
        while (true)
        {
            var result = await socket.ReceiveAsync(buffer, cancellationToken).ConfigureAwait(false);
            if (result.MessageType == WebSocketMessageType.Close)
            {
                return null;
            }
            stream.Write(buffer, 0, result.Count);
            if (stream.Length > 1024 * 1024)
            {
                throw new InvalidDataException("WebSocket message is too large.");
            }
            if (result.EndOfMessage)
            {
                return stream.ToArray();
            }
        }
    }

    private static string NormalizeServerUrl(string value)
    {
        var normalized = value.Trim().TrimEnd('/');
        _ = new FastCopyApiClient(normalized);
        return normalized;
    }

    private static void ValidateCredentials(string account, string password)
    {
        var accountLength = account.EnumerateRunes().Count();
        if (accountLength is < 1 or > 128 || account.Any(char.IsControl))
        {
            throw new ArgumentException("账号需为 1 至 128 个字符，且不能包含控制字符。");
        }
        var passwordLength = password.EnumerateRunes().Count();
        if (passwordLength is < 4 or > 256 || Encoding.UTF8.GetByteCount(password) > 1024)
        {
            throw new ArgumentException("密码需为 4 至 256 个字符。");
        }
    }

    private static string UserMessage(Exception exception) => exception switch
    {
        FastCopyApiException { Code: "INVALID_CREDENTIALS" } => "账号或密码不正确",
        FastCopyApiException { Code: "RATE_LIMITED" } => "尝试次数过多，请稍后再试",
        FastCopyApiException apiException => apiException.Message,
        HttpRequestException => "无法连接服务端，请检查网络",
        WebSocketException => "实时连接暂时不可用",
        CryptographicException => "加密状态无效，请重新登录",
        ArgumentException argumentException => argumentException.Message,
        _ => exception.Message
    };
}
