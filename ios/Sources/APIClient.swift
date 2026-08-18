import Foundation

struct AppAPIError: LocalizedError {
    let status: Int
    let code: String
    let message: String

    var errorDescription: String? { message }
    var unauthorized: Bool { status == 401 }
}

final class APIClient {
    private let baseURL: URL
    private let session: URLSession
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    init(baseURL: URL, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.session = session
    }

    func authenticate(_ body: AuthRequest) async throws -> AuthResponse {
        try await call("POST", path: "/v1/auth/session", body: body)
    }

    func refresh(_ refreshToken: String) async throws -> RefreshResponse {
        try await call("POST", path: "/v1/auth/refresh", body: RefreshRequest(refreshToken: refreshToken))
    }

    func devices(accessToken: String) async throws -> DevicesResponse {
        try await call("GET", path: "/v1/devices", accessToken: accessToken)
    }

    func upload(accessToken: String, clip: ClipUpload) async throws -> ClipCreateResponse {
        try await call("POST", path: "/v1/clips", accessToken: accessToken, body: clip)
    }

    func clips(accessToken: String, after sequence: Int64) async throws -> ClipsResponse {
        try await call("GET", path: "/v1/clips?after_seq=\(sequence)&limit=200", accessToken: accessToken)
    }

    func acknowledge(accessToken: String, sequence: Int64) async throws {
        try await callWithoutResponse(
            "POST",
            path: "/v1/sync/ack",
            accessToken: accessToken,
            body: AckRequest(seq: sequence)
        )
    }

    func revoke(accessToken: String, deviceID: String) async throws {
        try await callWithoutResponse(
            "POST",
            path: "/v1/devices/\(deviceID)/revoke",
            accessToken: accessToken
        )
    }

    func logout(accessToken: String) async throws {
        try await callWithoutResponse("POST", path: "/v1/auth/logout", accessToken: accessToken)
    }

    func registerAPNsToken(accessToken: String, token: String, environment: String) async throws {
        try await callWithoutResponse(
            "PUT",
            path: "/v1/push-tokens/apns",
            accessToken: accessToken,
            body: APNsTokenRequest(token: token, environment: environment)
        )
    }

    func deleteAPNsToken(accessToken: String) async throws {
        try await callWithoutResponse("DELETE", path: "/v1/push-tokens/apns", accessToken: accessToken)
    }

    func webSocket(
        accessToken: String,
        onConnected: @escaping () -> Void,
        onEvent: @escaping (SocketEvent) -> Void,
        onDisconnected: @escaping (Error?) -> Void
    ) throws -> WebSocketConnection {
        var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false)
        components?.scheme = baseURL.scheme == "https" ? "wss" : "ws"
        let basePath = baseURL.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        components?.path = basePath.isEmpty ? "/v1/events/ws" : "/\(basePath)/v1/events/ws"
        guard let url = components?.url else { throw URLError(.badURL) }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        request.setValue("ClipboardAssistantIOS/0.1.0", forHTTPHeaderField: "User-Agent")
        return WebSocketConnection(
            request: request,
            session: session,
            decoder: decoder,
            onConnected: onConnected,
            onEvent: onEvent,
            onDisconnected: onDisconnected
        )
    }

    private func call<Response: Decodable>(
        _ method: String,
        path: String,
        accessToken: String? = nil
    ) async throws -> Response {
        try await call(method, path: path, accessToken: accessToken, bodyData: nil)
    }

    private func call<Response: Decodable, Body: Encodable>(
        _ method: String,
        path: String,
        accessToken: String? = nil,
        body: Body
    ) async throws -> Response {
        try await call(method, path: path, accessToken: accessToken, bodyData: encoder.encode(body))
    }

    private func call<Response: Decodable>(
        _ method: String,
        path: String,
        accessToken: String?,
        bodyData: Data?
    ) async throws -> Response {
        let (data, response) = try await session.data(for: request(method, path: path, accessToken: accessToken, body: bodyData))
        try validate(response, data: data)
        return try decoder.decode(Response.self, from: data)
    }

    private func callWithoutResponse(
        _ method: String,
        path: String,
        accessToken: String? = nil
    ) async throws {
        try await callWithoutResponse(method, path: path, accessToken: accessToken, bodyData: nil)
    }

    private func callWithoutResponse<Body: Encodable>(
        _ method: String,
        path: String,
        accessToken: String? = nil,
        body: Body
    ) async throws {
        try await callWithoutResponse(method, path: path, accessToken: accessToken, bodyData: encoder.encode(body))
    }

    private func callWithoutResponse(
        _ method: String,
        path: String,
        accessToken: String?,
        bodyData: Data?
    ) async throws {
        let (data, response) = try await session.data(for: request(method, path: path, accessToken: accessToken, body: bodyData))
        try validate(response, data: data)
    }

    private func request(_ method: String, path: String, accessToken: String?, body: Data?) throws -> URLRequest {
        guard let url = URL(string: baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + path) else {
            throw URLError(.badURL)
        }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.httpBody = body
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("ClipboardAssistantIOS/0.1.0", forHTTPHeaderField: "User-Agent")
        if body != nil { request.setValue("application/json; charset=utf-8", forHTTPHeaderField: "Content-Type") }
        if let accessToken { request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization") }
        return request
    }

    private func validate(_ response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else { throw URLError(.badServerResponse) }
        guard (200..<300).contains(http.statusCode) else {
            let envelope = try? decoder.decode(APIErrorEnvelope.self, from: data)
            throw AppAPIError(
                status: http.statusCode,
                code: envelope?.error.code ?? "HTTP_\(http.statusCode)",
                message: envelope?.error.message ?? "服务器请求失败"
            )
        }
    }
}

final class WebSocketConnection {
    private let task: URLSessionWebSocketTask
    private let decoder: JSONDecoder
    private let onConnected: () -> Void
    private let onEvent: (SocketEvent) -> Void
    private let onDisconnected: (Error?) -> Void
    private var receiveTask: Task<Void, Never>?
    private var pingTask: Task<Void, Never>?
    private var closed = false

    init(
        request: URLRequest,
        session: URLSession,
        decoder: JSONDecoder,
        onConnected: @escaping () -> Void,
        onEvent: @escaping (SocketEvent) -> Void,
        onDisconnected: @escaping (Error?) -> Void
    ) {
        task = session.webSocketTask(with: request)
        self.decoder = decoder
        self.onConnected = onConnected
        self.onEvent = onEvent
        self.onDisconnected = onDisconnected
        task.resume()
        receiveTask = Task { [weak self] in await self?.receiveLoop() }
        pingTask = Task { [weak self] in await self?.connectAndPingLoop() }
    }

    func close() {
        guard !closed else { return }
        closed = true
        receiveTask?.cancel()
        pingTask?.cancel()
        task.cancel(with: .normalClosure, reason: nil)
    }

    private func receiveLoop() async {
        do {
            while !Task.isCancelled {
                let message = try await task.receive()
                let data: Data
                switch message {
                case let .data(value): data = value
                case let .string(value): data = Data(value.utf8)
                @unknown default: continue
                }
                if let event = try? decoder.decode(SocketEvent.self, from: data) { onEvent(event) }
            }
        } catch {
            if !closed { onDisconnected(error) }
        }
    }

    private func connectAndPingLoop() async {
        do {
            try await sendPing()
            guard !closed else { return }
            onConnected()
        } catch {
            if !closed { onDisconnected(error) }
            return
        }
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(25))
            guard !Task.isCancelled else { return }
            do {
                try await sendPing()
            } catch {
                if !closed { onDisconnected(error) }
                return
            }
        }
    }

    private func sendPing() async throws {
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            task.sendPing { error in
                if let error { continuation.resume(throwing: error) }
                else { continuation.resume() }
            }
        }
    }
}
