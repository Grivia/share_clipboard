using System.Drawing;
using FastCopy.Core;

namespace FastCopy.Windows;

internal sealed class SettingsForm : Form
{
    private readonly SyncEngine _engine;
    private readonly Panel _loginPanel = new();
    private readonly Panel _accountPanel = new();
    private readonly TextBox _serverInput = new();
    private readonly TextBox _accountInput = new();
    private readonly TextBox _passwordInput = new();
    private readonly Button _loginButton = new();
    private readonly Label _statusValue = new();
    private readonly Label _accountValue = new();
    private readonly Label _serverValue = new();
    private readonly Label _errorLabel = new();
    private readonly CheckBox _syncToggle = new();
    private readonly Button _syncButton = new();
    private readonly Button _logoutButton = new();
    private readonly ListView _devicesList = new();
    private readonly Button _refreshDevicesButton = new();
    private readonly Button _revokeButton = new();
    private bool _updating;
    private bool _allowClose;
    private bool _loginFieldsInitialized;
    private bool _previouslyAuthenticated;
    private bool _lastBusy;

    public SettingsForm(SyncEngine engine)
    {
        _engine = engine;
        Text = "粘贴板助手";
        Icon = SystemIcons.Application;
        StartPosition = FormStartPosition.CenterScreen;
        MinimumSize = new Size(520, 500);
        ClientSize = new Size(520, 500);
        MaximizeBox = false;
        Font = new Font("Segoe UI", 9F);
        AutoScaleMode = AutoScaleMode.Dpi;
        BackColor = SystemColors.Window;

        BuildLoginPanel();
        BuildAccountPanel();
        Controls.Add(_loginPanel);
        Controls.Add(_accountPanel);

        FormClosing += HandleFormClosing;
        VisibleChanged += (_, _) => _engine.SetDeviceViewActive(Visible);
        _engine.SnapshotChanged += HandleSnapshotChanged;
        ApplySnapshot(_engine.Snapshot);
    }

    public void ShowAndActivate()
    {
        if (!Visible)
        {
            Show();
        }
        if (WindowState == FormWindowState.Minimized)
        {
            WindowState = FormWindowState.Normal;
        }
        Activate();
        BringToFront();
    }

    public void CloseForExit()
    {
        _allowClose = true;
        Close();
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            _engine.SnapshotChanged -= HandleSnapshotChanged;
        }
        base.Dispose(disposing);
    }

    private void BuildLoginPanel()
    {
        _loginPanel.Dock = DockStyle.Fill;
        _loginPanel.Padding = new Padding(28, 24, 28, 24);

        var layout = new TableLayoutPanel
        {
            Dock = DockStyle.Top,
            AutoSize = true,
            ColumnCount = 2,
            RowCount = 7
        };
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 78));
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));

        var title = new Label
        {
            Text = "粘贴板助手",
            AutoSize = true,
            Font = new Font(Font.FontFamily, 18F, FontStyle.Bold),
            Margin = new Padding(0, 0, 0, 6)
        };
        var subtitle = new Label
        {
            Text = "登录或注册后，文本剪贴板会在设备间自动同步。",
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Margin = new Padding(0, 0, 0, 22)
        };
        layout.Controls.Add(title, 0, 0);
        layout.SetColumnSpan(title, 2);
        layout.Controls.Add(subtitle, 0, 1);
        layout.SetColumnSpan(subtitle, 2);

        ConfigureInput(_serverInput);
        ConfigureInput(_accountInput);
        ConfigureInput(_passwordInput);
        _passwordInput.UseSystemPasswordChar = true;
        _serverInput.TextChanged += (_, _) => UpdateLoginButton();
        _accountInput.TextChanged += (_, _) => UpdateLoginButton();
        _passwordInput.TextChanged += (_, _) => UpdateLoginButton();
        AddInputRow(layout, 2, "服务端", _serverInput);
        AddInputRow(layout, 3, "账号", _accountInput);
        AddInputRow(layout, 4, "密码", _passwordInput);

        _errorLabel.AutoSize = true;
        _errorLabel.ForeColor = Color.Firebrick;
        _errorLabel.MaximumSize = new Size(430, 0);
        _errorLabel.Margin = new Padding(0, 10, 0, 10);
        layout.Controls.Add(_errorLabel, 0, 5);
        layout.SetColumnSpan(_errorLabel, 2);

        _loginButton.Text = "登录或注册";
        _loginButton.AutoSize = true;
        _loginButton.Padding = new Padding(12, 4, 12, 4);
        _loginButton.Anchor = AnchorStyles.Right;
        _loginButton.Click += async (_, _) => await LoginAsync();
        layout.Controls.Add(_loginButton, 1, 6);
        AcceptButton = _loginButton;
        _loginPanel.Controls.Add(layout);
    }

    private void BuildAccountPanel()
    {
        _accountPanel.Dock = DockStyle.Fill;
        _accountPanel.Padding = new Padding(16);

        var tabs = new TabControl { Dock = DockStyle.Fill };
        var accountTab = new TabPage("账户") { Padding = new Padding(18), BackColor = SystemColors.Window };
        var devicesTab = new TabPage("设备") { Padding = new Padding(12), BackColor = SystemColors.Window };
        tabs.TabPages.Add(accountTab);
        tabs.TabPages.Add(devicesTab);

        var accountLayout = new TableLayoutPanel
        {
            Dock = DockStyle.Top,
            AutoSize = true,
            ColumnCount = 2,
            RowCount = 7
        };
        accountLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 82));
        accountLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        AddValueRow(accountLayout, 0, "状态", _statusValue);
        AddValueRow(accountLayout, 1, "账号", _accountValue);
        AddValueRow(accountLayout, 2, "服务端", _serverValue);

        _syncToggle.Text = "同步文本剪贴板";
        _syncToggle.AutoSize = true;
        _syncToggle.Margin = new Padding(0, 16, 0, 12);
        _syncToggle.CheckedChanged += async (_, _) =>
        {
            if (!_updating)
            {
                await _engine.SetSyncEnabledAsync(_syncToggle.Checked);
            }
        };
        accountLayout.Controls.Add(_syncToggle, 1, 3);

        var accountError = new Label
        {
            AutoSize = true,
            ForeColor = Color.Firebrick,
            MaximumSize = new Size(390, 0),
            Margin = new Padding(0, 6, 0, 10),
            Name = "AccountError"
        };
        accountLayout.Controls.Add(accountError, 0, 4);
        accountLayout.SetColumnSpan(accountError, 2);

        var pendingLabel = new Label
        {
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Margin = new Padding(0, 4, 0, 14),
            Name = "PendingLabel"
        };
        accountLayout.Controls.Add(pendingLabel, 0, 5);
        accountLayout.SetColumnSpan(pendingLabel, 2);

        var buttons = new FlowLayoutPanel
        {
            AutoSize = true,
            FlowDirection = FlowDirection.RightToLeft,
            Dock = DockStyle.Fill,
            WrapContents = false
        };
        _syncButton.Text = "立即同步";
        _syncButton.AutoSize = true;
        _syncButton.Click += async (_, _) => await _engine.RefreshNowAsync();
        _logoutButton.Text = "退出登录";
        _logoutButton.AutoSize = true;
        _logoutButton.Click += async (_, _) => await _engine.LogoutAsync();
        buttons.Controls.Add(_syncButton);
        buttons.Controls.Add(_logoutButton);
        accountLayout.Controls.Add(buttons, 0, 6);
        accountLayout.SetColumnSpan(buttons, 2);
        accountTab.Controls.Add(accountLayout);

        _devicesList.Dock = DockStyle.Fill;
        _devicesList.View = View.Details;
        _devicesList.FullRowSelect = true;
        _devicesList.MultiSelect = false;
        _devicesList.HideSelection = false;
        _devicesList.Columns.Add("设备", 170);
        _devicesList.Columns.Add("平台", 80);
        _devicesList.Columns.Add("状态", 75);
        _devicesList.Columns.Add("最近登录", 120);
        _devicesList.SelectedIndexChanged += (_, _) => UpdateRevokeButton();

        var deviceButtons = new FlowLayoutPanel
        {
            Dock = DockStyle.Bottom,
            Height = 38,
            FlowDirection = FlowDirection.RightToLeft,
            WrapContents = false,
            Padding = new Padding(0, 6, 0, 0)
        };
        _refreshDevicesButton.Text = "刷新";
        _refreshDevicesButton.AutoSize = true;
        _refreshDevicesButton.Click += async (_, _) => await _engine.RefreshDevicesAsync();
        _revokeButton.Text = "移除设备";
        _revokeButton.AutoSize = true;
        _revokeButton.Enabled = false;
        _revokeButton.Click += async (_, _) => await RevokeSelectedDeviceAsync();
        deviceButtons.Controls.Add(_refreshDevicesButton);
        deviceButtons.Controls.Add(_revokeButton);
        devicesTab.Controls.Add(_devicesList);
        devicesTab.Controls.Add(deviceButtons);

        _accountPanel.Controls.Add(tabs);
    }

    private async Task LoginAsync()
    {
        await _engine.AuthenticateAsync(
            _serverInput.Text,
            _accountInput.Text,
            _passwordInput.Text);
        if (_engine.Snapshot.IsAuthenticated)
        {
            _passwordInput.Clear();
        }
    }

    private async Task RevokeSelectedDeviceAsync()
    {
        if (_devicesList.SelectedItems.Count != 1
            || _devicesList.SelectedItems[0].Tag is not DeviceModel device
            || device.Current)
        {
            return;
        }
        var answer = MessageBox.Show(
            $"移除设备“{device.DisplayName}”？该设备需要重新登录才能同步。",
            "移除设备",
            MessageBoxButtons.OKCancel,
            MessageBoxIcon.Warning,
            MessageBoxDefaultButton.Button2);
        if (answer == DialogResult.OK)
        {
            await _engine.RevokeDeviceAsync(device);
        }
    }

    private void HandleSnapshotChanged(object? sender, ClientSnapshot snapshot)
    {
        ApplySnapshot(snapshot);
    }

    private void ApplySnapshot(ClientSnapshot snapshot)
    {
        _updating = true;
        try
        {
            _loginPanel.Visible = !snapshot.IsAuthenticated;
            _accountPanel.Visible = snapshot.IsAuthenticated;
            if (!_loginFieldsInitialized || (_previouslyAuthenticated && !snapshot.IsAuthenticated))
            {
                _serverInput.Text = snapshot.ServerUrl;
                _accountInput.Text = snapshot.Account;
                _loginFieldsInitialized = true;
            }
            _previouslyAuthenticated = snapshot.IsAuthenticated;
            _lastBusy = snapshot.IsBusy;
            UpdateLoginButton();
            _serverInput.Enabled = !snapshot.IsBusy;
            _accountInput.Enabled = !snapshot.IsBusy;
            _passwordInput.Enabled = !snapshot.IsBusy;
            _loginButton.Text = snapshot.IsBusy ? "正在登录..." : "登录或注册";
            _errorLabel.Text = snapshot.Error ?? "";

            _statusValue.Text = snapshot.Status;
            _statusValue.ForeColor = snapshot.IsConnected ? Color.ForestGreen : SystemColors.ControlText;
            _accountValue.Text = snapshot.Account;
            _serverValue.Text = snapshot.ServerUrl;
            _syncToggle.Checked = snapshot.SyncEnabled;
            _syncToggle.Enabled = !snapshot.IsBusy;
            _syncButton.Enabled = snapshot.SyncEnabled && !snapshot.IsBusy;
            _logoutButton.Enabled = !snapshot.IsBusy;

            var accountError = _accountPanel.Controls.Find("AccountError", true).OfType<Label>().First();
            accountError.Text = snapshot.Error ?? "";
            var pendingLabel = _accountPanel.Controls.Find("PendingLabel", true).OfType<Label>().First();
            pendingLabel.Text = snapshot.PendingCount == 0
                ? "没有待发送内容"
                : $"{snapshot.PendingCount} 条内容等待发送";
            ApplyDevices(snapshot.Devices);
        }
        finally
        {
            _updating = false;
        }
    }

    private void UpdateLoginButton()
    {
        _loginButton.Enabled = !_lastBusy
            && !string.IsNullOrWhiteSpace(_serverInput.Text)
            && !string.IsNullOrWhiteSpace(_accountInput.Text)
            && _passwordInput.TextLength >= 4;
    }

    private void ApplyDevices(IReadOnlyList<DeviceModel> devices)
    {
        var selectedId = _devicesList.SelectedItems.Count == 1
            && _devicesList.SelectedItems[0].Tag is DeviceModel selected
                ? selected.Id
                : null;
        _devicesList.BeginUpdate();
        try
        {
            _devicesList.Items.Clear();
            foreach (var device in devices)
            {
                var status = device.Current
                    ? "本机"
                    : device.Online
                        ? "在线"
                        : device.LoggedIn
                            ? "已登录"
                            : "已退出";
                var item = new ListViewItem(device.DisplayName)
                {
                    Tag = device,
                    ForeColor = device.Online ? Color.ForestGreen : SystemColors.ControlText
                };
                item.SubItems.Add(PlatformName(device.Platform));
                item.SubItems.Add(status);
                item.SubItems.Add(FormatTime(device.LastLoginAt));
                _devicesList.Items.Add(item);
                if (device.Id == selectedId)
                {
                    item.Selected = true;
                }
            }
        }
        finally
        {
            _devicesList.EndUpdate();
        }
        UpdateRevokeButton();
    }

    private void UpdateRevokeButton()
    {
        _revokeButton.Enabled = _devicesList.SelectedItems.Count == 1
            && _devicesList.SelectedItems[0].Tag is DeviceModel device
            && !device.Current
            && device.RevokedAt is null;
    }

    private void HandleFormClosing(object? sender, FormClosingEventArgs eventArgs)
    {
        if (_allowClose || eventArgs.CloseReason == CloseReason.WindowsShutDown)
        {
            return;
        }
        eventArgs.Cancel = true;
        Hide();
    }

    private static void ConfigureInput(TextBox input)
    {
        input.Dock = DockStyle.Fill;
        input.Margin = new Padding(0, 4, 0, 8);
    }

    private static void AddInputRow(
        TableLayoutPanel layout,
        int row,
        string labelText,
        Control input)
    {
        var label = new Label
        {
            Text = labelText,
            AutoSize = true,
            Anchor = AnchorStyles.Left,
            Margin = new Padding(0, 7, 10, 8)
        };
        layout.Controls.Add(label, 0, row);
        layout.Controls.Add(input, 1, row);
    }

    private static void AddValueRow(
        TableLayoutPanel layout,
        int row,
        string labelText,
        Label value)
    {
        var label = new Label
        {
            Text = labelText,
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Margin = new Padding(0, 7, 10, 9)
        };
        value.AutoSize = true;
        value.MaximumSize = new Size(340, 0);
        value.Margin = new Padding(0, 7, 0, 9);
        layout.Controls.Add(label, 0, row);
        layout.Controls.Add(value, 1, row);
    }

    private static string PlatformName(string platform) => platform switch
    {
        "windows" => "Windows",
        "macos" => "macOS",
        "android" => "Android",
        "ios" => "iOS",
        "linux" => "Linux",
        _ => platform
    };

    private static string FormatTime(string value) =>
        DateTimeOffset.TryParse(value, out var date)
            ? date.ToLocalTime().ToString("MM-dd HH:mm")
            : value;
}
