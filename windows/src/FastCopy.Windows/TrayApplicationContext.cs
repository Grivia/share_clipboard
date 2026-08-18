using System.Drawing;

namespace FastCopy.Windows;

internal sealed class TrayApplicationContext : ApplicationContext
{
    private readonly ClipboardWatcher _clipboard = new();
    private readonly SyncEngine _engine;
    private readonly SettingsForm _settingsForm;
    private readonly NotifyIcon _notifyIcon;
    private readonly ToolStripMenuItem _statusItem;
    private readonly ToolStripMenuItem _syncToggleItem;
    private readonly ToolStripMenuItem _syncNowItem;
    private bool _updatingMenu;
    private bool _exiting;
    private bool _engineDisposed;

    public TrayApplicationContext()
    {
        _engine = new SyncEngine(new LocalStore(), _clipboard);
        _settingsForm = new SettingsForm(_engine);

        _statusItem = new ToolStripMenuItem("尚未登录") { Enabled = false };
        _syncToggleItem = new ToolStripMenuItem("同步剪贴板") { CheckOnClick = true };
        _syncNowItem = new ToolStripMenuItem("立即同步");
        var openItem = new ToolStripMenuItem("打开 FastCopy");
        var exitItem = new ToolStripMenuItem("退出");

        openItem.Click += (_, _) => _settingsForm.ShowAndActivate();
        _syncToggleItem.CheckedChanged += async (_, _) =>
        {
            if (!_updatingMenu)
            {
                await _engine.SetSyncEnabledAsync(_syncToggleItem.Checked);
            }
        };
        _syncNowItem.Click += async (_, _) => await _engine.RefreshNowAsync();
        exitItem.Click += async (_, _) => await ExitAsync();

        var menu = new ContextMenuStrip();
        menu.Items.Add(_statusItem);
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add(openItem);
        menu.Items.Add(_syncNowItem);
        menu.Items.Add(_syncToggleItem);
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add(exitItem);

        _notifyIcon = new NotifyIcon
        {
            Icon = SystemIcons.Application,
            Text = "FastCopy",
            ContextMenuStrip = menu,
            Visible = true
        };
        _notifyIcon.MouseClick += (_, eventArgs) =>
        {
            if (eventArgs.Button == MouseButtons.Left)
            {
                _settingsForm.ShowAndActivate();
            }
        };

        _engine.SnapshotChanged += HandleSnapshotChanged;
        ApplySnapshot(_engine.Snapshot);

        EventHandler? startHandler = null;
        startHandler = async (_, _) =>
        {
            Application.Idle -= startHandler;
            await _engine.StartAsync();
            if (!_engine.Snapshot.IsAuthenticated)
            {
                _settingsForm.ShowAndActivate();
            }
        };
        Application.Idle += startHandler;
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            if (!_engineDisposed)
            {
                _engine.DisposeAsync().AsTask().GetAwaiter().GetResult();
                _engineDisposed = true;
            }
            _engine.SnapshotChanged -= HandleSnapshotChanged;
            _notifyIcon.Visible = false;
            _notifyIcon.Dispose();
            _settingsForm.Dispose();
        }
        base.Dispose(disposing);
    }

    private void HandleSnapshotChanged(object? sender, ClientSnapshot snapshot)
    {
        ApplySnapshot(snapshot);
    }

    private void ApplySnapshot(ClientSnapshot snapshot)
    {
        _updatingMenu = true;
        try
        {
            _statusItem.Text = snapshot.Status;
            _syncToggleItem.Checked = snapshot.SyncEnabled;
            _syncToggleItem.Enabled = snapshot.IsAuthenticated && !snapshot.IsBusy;
            _syncNowItem.Enabled = snapshot.IsAuthenticated && snapshot.SyncEnabled && !snapshot.IsBusy;
            var tooltip = snapshot.IsAuthenticated
                ? $"FastCopy - {snapshot.Status}"
                : "FastCopy - 尚未登录";
            _notifyIcon.Text = tooltip.Length <= 63 ? tooltip : tooltip[..63];
        }
        finally
        {
            _updatingMenu = false;
        }
    }

    private async Task ExitAsync()
    {
        if (_exiting)
        {
            return;
        }
        _exiting = true;
        _notifyIcon.Visible = false;
        _settingsForm.CloseForExit();
        await _engine.DisposeAsync();
        _engineDisposed = true;
        ExitThread();
    }
}
