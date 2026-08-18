using System.ComponentModel;
using System.Runtime.InteropServices;

namespace FastCopy.Windows;

internal sealed class ClipboardWatcher : NativeWindow, IDisposable
{
    private const int WmClipboardUpdate = 0x031D;
    private SynchronizationContext? _uiContext;
    private string? _lastText;
    private bool _started;
    private int _readGeneration;

    public event EventHandler<string>? TextChanged;

    public void Start()
    {
        if (_started)
        {
            return;
        }
        _uiContext = SynchronizationContext.Current
            ?? throw new InvalidOperationException("Clipboard watcher must start on the UI thread.");
        CreateHandle(new CreateParams { Caption = "FastCopy.ClipboardWatcher" });
        if (!AddClipboardFormatListener(Handle))
        {
            var error = new Win32Exception(Marshal.GetLastWin32Error());
            DestroyHandle();
            throw error;
        }
        _lastText = TryReadText();
        _started = true;
    }

    public void Stop()
    {
        if (!_started)
        {
            return;
        }
        RemoveClipboardFormatListener(Handle);
        DestroyHandle();
        _started = false;
    }

    public Task WriteWithoutUploadingAsync(string text, CancellationToken cancellationToken)
    {
        if (_uiContext is null)
        {
            return Task.CompletedTask;
        }

        var completion = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        _uiContext.Post(async _ =>
        {
            try
            {
                for (var attempt = 0; attempt < 5; attempt++)
                {
                    cancellationToken.ThrowIfCancellationRequested();
                    try
                    {
                        Clipboard.SetText(text, TextDataFormat.UnicodeText);
                        _lastText = text;
                        completion.TrySetResult();
                        return;
                    }
                    catch (ExternalException) when (attempt < 4)
                    {
                        await Task.Delay(40 * (attempt + 1), cancellationToken);
                    }
                }
            }
            catch (Exception exception)
            {
                completion.TrySetException(exception);
            }
        }, null);
        return completion.Task;
    }

    protected override void WndProc(ref Message message)
    {
        if (message.Msg == WmClipboardUpdate)
        {
            ReadClipboardUpdate();
        }
        base.WndProc(ref message);
    }

    public void Dispose()
    {
        Stop();
        GC.SuppressFinalize(this);
    }

    private static string? TryReadText()
    {
        try
        {
            return Clipboard.ContainsText(TextDataFormat.UnicodeText)
                ? Clipboard.GetText(TextDataFormat.UnicodeText)
                : null;
        }
        catch (Exception exception) when (exception is ExternalException or ThreadStateException)
        {
            return null;
        }
    }

    private void ReadClipboardUpdate()
    {
        var generation = Interlocked.Increment(ref _readGeneration);
        var text = TryReadText();
        if (text is not null)
        {
            NotifyIfChanged(text);
            return;
        }

        _uiContext?.Post(async _ =>
        {
            for (var attempt = 1; attempt <= 5; attempt++)
            {
                await Task.Delay(40 * attempt);
                if (generation != Volatile.Read(ref _readGeneration) || !_started)
                {
                    return;
                }
                var retriedText = TryReadText();
                if (retriedText is not null)
                {
                    NotifyIfChanged(retriedText);
                    return;
                }
            }
        }, null);
    }

    private void NotifyIfChanged(string text)
    {
        if (string.Equals(text, _lastText, StringComparison.Ordinal))
        {
            return;
        }
        _lastText = text;
        TextChanged?.Invoke(this, text);
    }

    [DllImport("user32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool AddClipboardFormatListener(IntPtr windowHandle);

    [DllImport("user32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool RemoveClipboardFormatListener(IntPtr windowHandle);
}
