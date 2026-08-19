import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        Group {
            if model.isAuthenticated {
                authenticatedView
            } else {
                authenticationView
            }
        }
        .padding(20)
    }

    private var authenticationView: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack(spacing: 10) {
                Image(systemName: "clipboard.fill")
                    .font(.title2)
                Text("粘贴板助手")
                    .font(.title2.weight(.semibold))
            }

            Form {
                TextField("服务端", text: $model.serverURL, prompt: Text("https://zhy.hair/fastcopy"))
                TextField("账号", text: $model.account)
                SecureField("密码", text: $model.password)
            }
            .formStyle(.grouped)

            if let error = model.errorText {
                Label(error, systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.red)
                    .font(.callout)
                    .fixedSize(horizontal: false, vertical: true)
            }

            HStack {
                Spacer()
                Button {
                    Task { await model.authenticate() }
                } label: {
                    if model.isBusy {
                        ProgressView()
                            .controlSize(.small)
                    } else {
                        Label("登录或注册", systemImage: "person.crop.circle.badge.checkmark")
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(
                    model.isBusy
                        || model.account.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                        || model.password.count < 4
                )
            }
        }
    }

    private var authenticatedView: some View {
        TabView {
            accountView
                .tabItem { Label("账户", systemImage: "person.crop.circle") }
            devicesView
                .tabItem { Label("设备", systemImage: "laptopcomputer.and.iphone") }
        }
    }

    private var accountView: some View {
        VStack(alignment: .leading, spacing: 16) {
            Form {
                Section("连接") {
                    LabeledContent("状态") {
                        Label(
                            model.statusText,
                            systemImage: model.isConnected ? "checkmark.circle.fill" : "circle.dotted"
                        )
                        .foregroundStyle(model.isConnected ? .green : .secondary)
                    }
                    LabeledContent("账号", value: model.authenticatedAccount)
                    LabeledContent("服务端", value: model.serverURL)
                    Toggle("同步剪贴板", isOn: $model.syncEnabled)
                }
            }
            .formStyle(.grouped)

            if let error = model.errorText {
                Label(error, systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.red)
                    .font(.callout)
            }

            HStack {
                Button(role: .destructive) {
                    Task { await model.logout() }
                } label: {
                    Label("退出登录", systemImage: "rectangle.portrait.and.arrow.right")
                }
                Spacer()
                Button {
                    Task { await model.refreshNow() }
                } label: {
                    Label("立即同步", systemImage: "arrow.triangle.2.circlepath")
                }
            }
        }
        .padding(.top, 8)
    }

    private var devicesView: some View {
        VStack(spacing: 12) {
            List(model.devices) { device in
                HStack(spacing: 12) {
                    Image(systemName: platformIcon(device.platform))
                        .frame(width: 22)

                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: 6) {
                            Text(device.displayName)
                                .lineLimit(1)
                            if device.current {
                                Text("本机")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            if device.role != nil {
                                Text(device.roleLabel)
                                    .font(.caption)
                                    .foregroundStyle(device.role == "super_admin" ? .orange : .secondary)
                            }
                        }
                        Text(device.loggedIn ? (device.online ? "在线" : "已登录") : "已退出")
                            .font(.caption)
                            .foregroundStyle(device.online ? .green : .secondary)
                    }

                    Spacer()

                    if device.canRevoke == true || device.canChangeRole == true {
                        Menu {
                            if device.canChangeRole == true {
                                Button {
                                    let role = device.role == "admin" ? "member" : "admin"
                                    Task { await model.setRole(device, role: role) }
                                } label: {
                                    Label(
                                        device.role == "admin" ? "取消管理员" : "设为管理员",
                                        systemImage: device.role == "admin" ? "person.badge.minus" : "person.badge.plus"
                                    )
                                }
                            }
                            if device.canRevoke == true {
                                Button(role: .destructive) {
                                    Task { await model.revoke(device) }
                                } label: {
                                    Label("移除此设备", systemImage: "xmark.circle")
                                }
                            }
                        } label: {
                            Image(systemName: "ellipsis.circle")
                        }
                        .menuStyle(.borderlessButton)
                        .fixedSize()
                        .help("管理设备")
                    }
                }
                .padding(.vertical, 4)
            }

            HStack {
                Text("历史设备与当前在线状态")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button {
                    Task { await model.refreshDevices() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .buttonStyle(.borderless)
                .help("刷新设备列表")
            }
        }
        .padding(.top, 8)
    }

    private func platformIcon(_ platform: String) -> String {
        switch platform {
        case "macos": return "laptopcomputer"
        case "android": return "smartphone"
        case "ios": return "iphone"
        case "windows": return "desktopcomputer"
        default: return "terminal"
        }
    }
}
