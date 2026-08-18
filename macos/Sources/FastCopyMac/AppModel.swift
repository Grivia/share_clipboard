import AppKit
import Foundation

enum SyncTiming {
    static let connectedReconciliationNanoseconds: UInt64 = 5 * 60 * 1_000_000_000
    static let disconnectedReconciliationNanoseconds: UInt64 = 60 * 1_000_000_000
    private static let pendingRetrySeconds: [UInt64] = [2, 5, 15, 30, 60]

    static func pendingRetryNanoseconds(attempt: Int) -> UInt64 {
        let index = min(max(attempt, 0), pendingRetrySeconds.count - 1)
        return pendingRetrySeconds[index] * 1_000_000_000
    }
}

@MainActor
final class AppModel: ObservableObject {
    @Published var serverURL: String
    @Published var account: String
    @Published var password = ""
    @Published var syncEnabled: Bool {
        didSet {
            defaults.set(syncEnabled, forKey: Keys.syncEnabled)
            guard started else { return }
            Task { @MainActor [weak self] in
                guard let self else { return }
                if self.syncEnabled {
                    await self.startSyncServices()
                } else {
                    self.stopSyncServices()
                    self.statusText = "同步已暂停"
                }
            }
        }
    }

    @Published private(set) var isAuthenticated = false
    @Published private(set) var isConnected = false
    @Published private(set) var isBusy = false
    @Published private(set) var statusText = "尚未登录"
    @Published private(set) var errorText: String?
    @Published private(set) var devices: [Device] = []
    @Published private(set) var authenticatedAccount = ""

    private enum Keys {
        static let serverURL = "serverURL"
        static let account = "account"
        static let legacyEmail = "email"
        static let syncEnabled = "syncEnabled"
        static let userID = "userID"
        static let deviceID = "deviceID"
        static let pendingOwner = "pendingOwner"
        static let accessToken = "accessToken"
        static let refreshToken = "refreshToken"
        static let sharedKey = "sharedKey"
        static let keyDerivationVersion = "keyDerivationVersion"
        static let installationID = "installationID"
    }

    private let defaults = UserDefaults.standard
    private let secureStore = SecureStore()
    private let pendingStore = PendingUploadStore()
    private let clipboard = ClipboardMonitor()
    private var started = false
    private var accessToken: String?
    private var refreshToken: String?
    private var userID: String?
    private var deviceID: String?
    private var sharedKey: String
    private var pendingUploads: [ClipUpload]
    private var isFlushing = false
    private var isSynchronizing = false
    private var synchronizeAgain = false
    private var webSocket: URLSessionWebSocketTask?
    private var webSocketLoop: Task<Void, Never>?
    private var reconciliationTask: Task<Void, Never>?
    private var pendingRetryTask: Task<Void, Never>?
    private var pendingRetryAttempt = 0
    private var refreshTask: Task<SessionTokens, Error>?

    init() {
        serverURL = defaults.string(forKey: Keys.serverURL) ?? "https://zhy.hair/fastcopy"
        account = defaults.string(forKey: Keys.account) ?? defaults.string(forKey: Keys.legacyEmail) ?? ""
        sharedKey = secureStore.string(for: Keys.sharedKey) ?? ""
        accessToken = secureStore.string(for: Keys.accessToken)
        refreshToken = secureStore.string(for: Keys.refreshToken)
        userID = defaults.string(forKey: Keys.userID)
        deviceID = defaults.string(forKey: Keys.deviceID)
        pendingUploads = pendingStore.load()
        if defaults.object(forKey: Keys.syncEnabled) == nil {
            syncEnabled = true
        } else {
            syncEnabled = defaults.bool(forKey: Keys.syncEnabled)
        }
        let hasDerivedKey = defaults.integer(forKey: Keys.keyDerivationVersion) == KeyDerivation.version
            && (try? CryptoBox.normalizedKey(sharedKey)) != nil
        isAuthenticated = accessToken != nil && refreshToken != nil && userID != nil && deviceID != nil && hasDerivedKey
        authenticatedAccount = isAuthenticated ? account : ""
        if !isAuthenticated && (accessToken != nil || refreshToken != nil) {
            accessToken = nil
            refreshToken = nil
            userID = nil
            deviceID = nil
            sharedKey = ""
            pendingUploads.removeAll()
            pendingStore.clear()
            secureStore.delete(Keys.accessToken)
            secureStore.delete(Keys.refreshToken)
            secureStore.delete(Keys.sharedKey)
            defaults.removeObject(forKey: Keys.userID)
            defaults.removeObject(forKey: Keys.deviceID)
            defaults.removeObject(forKey: Keys.pendingOwner)
            defaults.removeObject(forKey: Keys.keyDerivationVersion)
        }
    }

    var statusIcon: String {
        if !isAuthenticated { return "clipboard" }
        if !syncEnabled { return "pause.circle" }
        return isConnected ? "clipboard.fill" : "icloud.slash"
    }

    func start() async {
        guard !started else { return }
        started = true
        if isAuthenticated {
            await startSyncServices()
            await refreshDevices()
        }
    }

    func authenticate() async {
        guard !isBusy else { return }
        isBusy = true
        errorText = nil
        defer { isBusy = false }

        do {
            let normalizedAccount = account.trimmingCharacters(in: .whitespacesAndNewlines)
            let enteredPassword = password
            let client = APIClient(baseURL: serverURL)
            let response = try await client.authenticate(
                request: AuthRequest(
                    account: normalizedAccount,
                    password: enteredPassword,
                    device: deviceInput()
                )
            )
            let canonicalAccount = response.user.account
            let derivedKey = try await Task.detached(priority: .userInitiated) {
                try KeyDerivation.derive(account: canonicalAccount, password: enteredPassword)
            }.value
            try acceptAuthentication(response, derivedKey: derivedKey)
            password = ""
            await startSyncServices()
            await refreshDevices()
        } catch {
            errorText = userMessage(error)
            statusText = "登录失败"
        }
    }

    func logout() async {
        if let token = accessToken {
            try? await APIClient(baseURL: serverURL).logout(token: token)
        }
        clearAuthentication()
    }

    func refreshNow() async {
        errorText = nil
        await flushPendingUploads()
        await synchronize()
        await refreshDevices()
        scheduleReconciliation()
    }

    func refreshDevices() async {
        guard isAuthenticated else { return }
        do {
            let response: DevicesResponse = try await authorized { client, token in
                try await client.devices(token: token)
            }
            devices = response.devices
        } catch {
            errorText = userMessage(error)
        }
    }

    func revoke(_ device: Device) async {
        guard !device.current else { return }
        do {
            try await authorized { client, token in
                try await client.revokeDevice(id: device.id, token: token)
            }
            await refreshDevices()
        } catch {
            errorText = userMessage(error)
        }
    }

    private func acceptAuthentication(_ response: AuthResponse, derivedKey: String) throws {
        do {
            try secureStore.set(response.tokens.accessToken, for: Keys.accessToken)
            try secureStore.set(response.tokens.refreshToken, for: Keys.refreshToken)
            try secureStore.set(derivedKey, for: Keys.sharedKey)
        } catch {
            secureStore.delete(Keys.accessToken)
            secureStore.delete(Keys.refreshToken)
            secureStore.delete(Keys.sharedKey)
            throw error
        }

        stopSyncServices()
        let previousOwner = defaults.string(forKey: Keys.pendingOwner)
        if previousOwner != nil && previousOwner != response.user.id {
            pendingUploads.removeAll()
            pendingStore.clear()
        }

        accessToken = response.tokens.accessToken
        refreshToken = response.tokens.refreshToken
        userID = response.user.id
        deviceID = response.device.id
        account = response.user.account
        authenticatedAccount = response.user.account
        sharedKey = derivedKey
        devices = [response.device]
        isAuthenticated = true
        statusText = "正在连接"

        defaults.set(serverURL.trimmingCharacters(in: .whitespacesAndNewlines), forKey: Keys.serverURL)
        defaults.set(account, forKey: Keys.account)
        defaults.removeObject(forKey: Keys.legacyEmail)
        defaults.set(userID, forKey: Keys.userID)
        defaults.set(deviceID, forKey: Keys.deviceID)
        defaults.set(userID, forKey: Keys.pendingOwner)
        defaults.set(KeyDerivation.version, forKey: Keys.keyDerivationVersion)
    }

    private func clearAuthentication() {
        stopSyncServices()
        accessToken = nil
        refreshToken = nil
        userID = nil
        deviceID = nil
        isAuthenticated = false
        isConnected = false
        authenticatedAccount = ""
        devices = []
        sharedKey = ""
        pendingUploads.removeAll()
        pendingStore.clear()
        defaults.removeObject(forKey: Keys.userID)
        defaults.removeObject(forKey: Keys.deviceID)
        defaults.removeObject(forKey: Keys.pendingOwner)
        defaults.removeObject(forKey: Keys.keyDerivationVersion)
        secureStore.delete(Keys.accessToken)
        secureStore.delete(Keys.refreshToken)
        secureStore.delete(Keys.sharedKey)
        statusText = "尚未登录"
    }

    private func startSyncServices() async {
        guard isAuthenticated, syncEnabled else { return }
        do {
            sharedKey = try CryptoBox.normalizedKey(sharedKey)
        } catch {
            errorText = userMessage(error)
            statusText = "加密状态无效，请重新登录"
            return
        }
        clipboard.start { [weak self] text in
            Task { @MainActor in await self?.localClipboardChanged(text) }
        }
        startWebSocketLoop()
        await flushPendingUploads()
        await synchronize()
        scheduleReconciliation()
    }

    private func stopSyncServices() {
        clipboard.stop()
        reconciliationTask?.cancel()
        reconciliationTask = nil
        clearPendingRetry()
        webSocketLoop?.cancel()
        webSocketLoop = nil
        webSocket?.cancel(with: .goingAway, reason: nil)
        webSocket = nil
        isConnected = false
    }

    private func localClipboardChanged(_ text: String) async {
        guard isAuthenticated, syncEnabled else { return }
        guard Data(text.utf8).count <= 256 * 1024 - 16 else {
            errorText = "文本超过 256 KiB，未同步"
            return
        }
        do {
            let eventID = UUID().uuidString.lowercased()
            let upload = try CryptoBox.encrypt(text, keyBase64: sharedKey, clientEventId: eventID)
            pendingUploads.append(upload)
            pendingUploads = Array(pendingUploads.suffix(100))
            pendingStore.save(pendingUploads)
            await flushPendingUploads()
        } catch {
            errorText = userMessage(error)
        }
    }

    private func flushPendingUploads() async {
        guard isAuthenticated, syncEnabled else { return }
        if pendingUploads.isEmpty {
            clearPendingRetry()
            return
        }
        guard !isFlushing else { return }
        isFlushing = true
        defer { isFlushing = false }

        while let upload = pendingUploads.first {
            do {
                _ = try await authorized { client, token in
                    try await client.upload(upload, token: token)
                } as ClipCreateResponse
                pendingUploads.removeFirst()
                pendingStore.save(pendingUploads)
                statusText = "剪贴板已同步"
            } catch {
                statusText = "等待网络恢复"
                errorText = userMessage(error)
                schedulePendingRetry()
                return
            }
        }
        clearPendingRetry()
    }

    private func schedulePendingRetry() {
        guard pendingRetryTask == nil, isAuthenticated, syncEnabled, !pendingUploads.isEmpty else { return }
        let delay = SyncTiming.pendingRetryNanoseconds(attempt: pendingRetryAttempt)
        pendingRetryAttempt += 1
        pendingRetryTask = Task { @MainActor [weak self] in
            do {
                try await Task.sleep(nanoseconds: delay)
            } catch {
                return
            }
            guard let self, !Task.isCancelled else { return }
            self.pendingRetryTask = nil
            await self.flushPendingUploads()
        }
    }

    private func clearPendingRetry() {
        pendingRetryTask?.cancel()
        pendingRetryTask = nil
        pendingRetryAttempt = 0
    }

    private func synchronize() async {
        guard isAuthenticated, syncEnabled else { return }
        if isSynchronizing {
            synchronizeAgain = true
            return
        }
        isSynchronizing = true
        defer {
            isSynchronizing = false
            if synchronizeAgain {
                synchronizeAgain = false
                Task { @MainActor [weak self] in await self?.synchronize() }
            }
        }

        let initialSeq = lastSequence
        var cursor = initialSeq
        do {
            while true {
                let response: ClipsResponse = try await authorized { client, token in
                    try await client.clips(afterSeq: cursor, token: token)
                }
                for event in response.clips {
                    cursor = max(cursor, event.seq)
                    guard event.originDeviceId != deviceID else { continue }
                    do {
                        let text = try CryptoBox.decrypt(event, keyBase64: sharedKey)
                        clipboard.writeWithoutUploading(text)
                        statusText = "已接收来自 \(event.originName) 的文本"
                    } catch {
                        errorText = "无法解密来自 \(event.originName) 的文本，请重新登录"
                    }
                }
                if response.clips.count < 200 { break }
            }
            if cursor > initialSeq {
                lastSequence = cursor
                try await authorized { client, token in
                    try await client.acknowledge(seq: cursor, token: token)
                }
            }
            if statusText == "正在连接" || statusText == "等待网络恢复" {
                statusText = "同步就绪"
            }
        } catch {
            statusText = "等待网络恢复"
            errorText = userMessage(error)
        }
    }

    private func startWebSocketLoop() {
        webSocketLoop?.cancel()
        webSocketLoop = Task { @MainActor [weak self] in
            await self?.runWebSocketLoop()
        }
    }

    private func scheduleReconciliation() {
        reconciliationTask?.cancel()
        reconciliationTask = nil
        guard isAuthenticated, syncEnabled else { return }
        let delay = isConnected
            ? SyncTiming.connectedReconciliationNanoseconds
            : SyncTiming.disconnectedReconciliationNanoseconds
        reconciliationTask = Task { @MainActor [weak self] in
            do {
                try await Task.sleep(nanoseconds: delay)
            } catch {
                return
            }
            guard let self, !Task.isCancelled, self.isAuthenticated, self.syncEnabled else { return }
            self.reconciliationTask = nil
            await self.flushPendingUploads()
            await self.synchronize()
            self.scheduleReconciliation()
        }
    }

    private func setConnectionState(_ connected: Bool) {
        guard isConnected != connected else { return }
        isConnected = connected
        scheduleReconciliation()
    }

    private func runWebSocketLoop() async {
        var delay: UInt64 = 1
        while !Task.isCancelled && isAuthenticated && syncEnabled {
            guard let token = accessToken else { return }
            do {
                let request = try APIClient(baseURL: serverURL).webSocketRequest(token: token)
                let socket = URLSession.shared.webSocketTask(with: request)
                webSocket = socket
                socket.resume()

                while !Task.isCancelled {
                    let message = try await socket.receive()
                    setConnectionState(true)
                    delay = 1
                    let data: Data
                    switch message {
                    case .data(let value): data = value
                    case .string(let value): data = Data(value.utf8)
                    @unknown default: continue
                    }
                    guard let envelope = try? JSONDecoder().decode(WebSocketEnvelope.self, from: data) else {
                        continue
                    }
                    switch envelope.type {
                    case "hello":
                        statusText = "同步就绪"
                        await flushPendingUploads()
                        await synchronize()
                    case "clip.created":
                        await synchronize()
                    case "device.logged_in", "device.updated", "device.revoked", "device.presence_changed":
                        await refreshDevices()
                    default:
                        break
                    }
                }
            } catch {
                setConnectionState(false)
                webSocket = nil
                if !Task.isCancelled {
                    statusText = "正在重新连接"
                    try? await Task.sleep(nanoseconds: delay * 1_000_000_000)
                    delay = min(delay * 2, 30)
                }
            }
        }
    }

    private func authorized<Result>(
        _ operation: (APIClient, String) async throws -> Result
    ) async throws -> Result {
        guard let token = accessToken else {
            throw APIClientError.server(status: 401, code: "NOT_AUTHENTICATED", message: "请重新登录")
        }
        let client = APIClient(baseURL: serverURL)
        do {
            return try await operation(client, token)
        } catch let error as APIClientError where error.isUnauthorized {
            do {
                try await refreshSession(client: client)
            } catch {
                clearAuthentication()
                throw error
            }
            guard let renewedToken = accessToken else {
                throw APIClientError.server(status: 401, code: "SESSION_EXPIRED", message: "登录已过期")
            }
            return try await operation(client, renewedToken)
        }
    }

    private func refreshSession(client: APIClient) async throws {
        if let refreshTask {
            let tokens = try await refreshTask.value
            try persist(tokens: tokens)
            return
        }
        guard let refreshToken else {
            throw APIClientError.server(status: 401, code: "SESSION_EXPIRED", message: "登录已过期")
        }
        let task = Task { try await client.refresh(refreshToken).tokens }
        refreshTask = task
        defer { refreshTask = nil }
        let tokens = try await task.value
        try persist(tokens: tokens)
    }

    private func persist(tokens: SessionTokens) throws {
        accessToken = tokens.accessToken
        refreshToken = tokens.refreshToken
        try secureStore.set(tokens.accessToken, for: Keys.accessToken)
        try secureStore.set(tokens.refreshToken, for: Keys.refreshToken)
    }

    private var lastSequence: Int64 {
        get {
            guard let userID else { return 0 }
            return Int64(defaults.string(forKey: "lastSequence.\(userID)") ?? "0") ?? 0
        }
        set {
            guard let userID else { return }
            defaults.set(String(newValue), forKey: "lastSequence.\(userID)")
        }
    }

    private func installationID() -> String {
        if let value = secureStore.string(for: Keys.installationID) {
            return value
        }
        if let value = defaults.string(forKey: Keys.installationID) {
            return value
        }
        let value = UUID().uuidString.lowercased()
        do {
            try secureStore.set(value, for: Keys.installationID)
        } catch {
            defaults.set(value, forKey: Keys.installationID)
        }
        return value
    }

    private func deviceInput() -> DeviceInput {
        let version = ProcessInfo.processInfo.operatingSystemVersion
        return DeviceInput(
            installationId: installationID(),
            reportedName: Host.current().localizedName ?? ProcessInfo.processInfo.hostName,
            platform: "macos",
            osVersion: "\(version.majorVersion).\(version.minorVersion).\(version.patchVersion)",
            appVersion: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.2.1"
        )
    }

    private func userMessage(_ error: Error) -> String {
        if let localized = error as? LocalizedError, let message = localized.errorDescription {
            return message
        }
        return error.localizedDescription
    }
}
