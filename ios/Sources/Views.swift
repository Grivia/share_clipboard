import SwiftUI

struct RootView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        Group {
            if !model.initialized {
                ProgressView()
            } else if !model.authenticated {
                LoginView()
            } else {
                MainView()
            }
        }
    }
}

private struct LoginView: View {
    @EnvironmentObject private var model: AppModel
    @State private var server = ""
    @State private var account = ""
    @State private var password = ""

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("服务端", text: $server)
                        .textInputAutocapitalization(.never)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                    TextField("账号", text: $account)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    SecureField("密码", text: $password)
                        .textContentType(.password)
                }
                if let errorMessage = model.errorMessage {
                    Section {
                        Label(errorMessage, systemImage: "exclamationmark.circle")
                            .foregroundStyle(.red)
                    }
                }
                Section {
                    Button {
                        Task { await model.authenticate(server: server, account: account, password: password) }
                    } label: {
                        HStack {
                            Spacer()
                            if model.busy {
                                ProgressView()
                            } else {
                                Text("登录或注册")
                                    .fontWeight(.semibold)
                            }
                            Spacer()
                        }
                    }
                    .disabled(model.busy || account.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || password.count < 4)
                }
            }
            .navigationTitle("粘贴板助手")
            .onAppear {
                if server.isEmpty { server = model.serverURL }
                if account.isEmpty { account = model.account }
            }
        }
    }
}

private struct MainView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        TabView {
            NavigationStack { SyncView() }
                .tabItem { Label("同步", systemImage: "arrow.triangle.2.circlepath") }
            NavigationStack { DevicesView() }
                .tabItem { Label("设备", systemImage: "macbook.and.iphone") }
            NavigationStack { SettingsView() }
                .tabItem { Label("设置", systemImage: "gearshape") }
        }
    }
}

private struct SyncView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        List {
            Section {
                HStack(spacing: 12) {
                    Image(systemName: model.connected ? "checkmark.icloud.fill" : "arrow.triangle.2.circlepath.icloud")
                        .font(.title2)
                        .foregroundStyle(model.connected ? Color.accentColor : .secondary)
                        .frame(width: 32)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(model.status)
                            .font(.headline)
                        Text(model.account)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                }
                if let errorMessage = model.errorMessage {
                    Label(errorMessage, systemImage: "exclamationmark.circle")
                        .foregroundStyle(.red)
                }
                if model.pendingCount > 0 {
                    LabeledContent("等待发送", value: "\(model.pendingCount)")
                }
            }

            Section("操作") {
                Button {
                    Task { await model.sendCurrentClipboard() }
                } label: {
                    Label("发送当前剪贴板", systemImage: "paperplane")
                }
                .disabled(!model.syncEnabled)

                Button {
                    model.copyLatest()
                } label: {
                    Label("复制最新内容", systemImage: "doc.on.doc")
                }
                .disabled(model.latestText == nil)
            }

            Section("最新内容") {
                if let latestText = model.latestText {
                    Text(latestText)
                        .textSelection(.enabled)
                        .lineLimit(12)
                    if let latestOrigin = model.latestOrigin {
                        LabeledContent("来源", value: latestOrigin)
                            .foregroundStyle(.secondary)
                    }
                } else {
                    ContentUnavailableView("暂无内容", systemImage: "doc.on.clipboard")
                }
            }
        }
        .navigationTitle("同步")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    Task { await model.refreshNow() }
                } label: {
                    if model.busy { ProgressView() }
                    else { Image(systemName: "arrow.clockwise") }
                }
                .disabled(model.busy)
                .accessibilityLabel("立即刷新")
            }
        }
    }
}

private struct DevicesView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        List {
            if model.devices.isEmpty {
                ContentUnavailableView("暂无设备", systemImage: "macbook.and.iphone")
            } else {
                ForEach(model.devices) { device in
                    DeviceRow(device: device)
                        .swipeActions {
                            if !device.current, device.revokedAt == nil {
                                Button("撤销", role: .destructive) {
                                    Task { await model.revoke(device) }
                                }
                            }
                        }
                }
            }
        }
        .navigationTitle("设备")
        .refreshable { await model.refreshDevicesNow() }
        .task { await model.refreshDevicesNow() }
    }
}

private struct DeviceRow: View {
    let device: DeviceModel

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: platformIcon)
                .font(.title3)
                .foregroundStyle(.secondary)
                .frame(width: 30)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(device.displayName)
                    if device.current { Text("本机").font(.caption).foregroundStyle(.secondary) }
                }
                Text("\(device.platform) · \(device.osVersion)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            Circle()
                .fill(device.online ? Color.green : Color.secondary.opacity(0.35))
                .frame(width: 8, height: 8)
                .accessibilityLabel(device.online ? "在线" : "离线")
        }
        .contentShape(Rectangle())
    }

    private var platformIcon: String {
        switch device.platform.lowercased() {
        case "ios": return "iphone"
        case "android": return "apps.iphone"
        case "macos": return "macbook"
        case "windows", "linux": return "desktopcomputer"
        default: return "display"
        }
    }
}

private struct SettingsView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        Form {
            Section {
                Toggle(
                    "剪贴板同步",
                    isOn: Binding(
                        get: { model.syncEnabled },
                        set: model.setSyncEnabled
                    )
                )
            }
            Section("账号") {
                LabeledContent("账号", value: model.account)
                LabeledContent("服务端", value: model.serverURL)
            }
            Section {
                Button("退出登录", role: .destructive) {
                    Task { await model.logout() }
                }
            }
        }
        .navigationTitle("设置")
    }
}
