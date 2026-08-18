import Foundation

enum APIClientError: LocalizedError {
    case invalidServerURL
    case invalidResponse
    case server(status: Int, code: String, message: String)

    var isUnauthorized: Bool {
        if case .server(let status, _, _) = self {
            return status == 401
        }
        return false
    }

    var errorDescription: String? {
        switch self {
        case .invalidServerURL:
            return "服务端地址无效"
        case .invalidResponse:
            return "服务端返回了无法识别的数据"
        case .server(_, _, let message):
            return message
        }
    }
}

struct APIClient {
    let baseURL: String
    private let session: URLSession
    private static let userAgent = "FastCopyMac/\(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.2.1")"

    init(baseURL: String, session: URLSession = .shared) {
        self.baseURL = baseURL.trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        self.session = session
    }

    func authenticate(request: AuthRequest) async throws -> AuthResponse {
        try await send(
            AuthResponse.self,
            method: "POST",
            path: "/v1/auth/session",
            body: encode(request)
        )
    }

    func refresh(_ refreshToken: String) async throws -> RefreshResponse {
        struct Request: Encodable { let refreshToken: String }
        return try await send(
            RefreshResponse.self,
            method: "POST",
            path: "/v1/auth/refresh",
            body: encode(Request(refreshToken: refreshToken))
        )
    }

    func logout(token: String) async throws {
        try await sendWithoutResponse(method: "POST", path: "/v1/auth/logout", token: token)
    }

    func upload(_ upload: ClipUpload, token: String) async throws -> ClipCreateResponse {
        try await send(
            ClipCreateResponse.self,
            method: "POST",
            path: "/v1/clips",
            body: encode(upload),
            token: token
        )
    }

    func clips(afterSeq: Int64, token: String) async throws -> ClipsResponse {
        try await send(
            ClipsResponse.self,
            method: "GET",
            path: "/v1/clips",
            queryItems: [
                URLQueryItem(name: "after_seq", value: String(afterSeq)),
                URLQueryItem(name: "limit", value: "200")
            ],
            token: token
        )
    }

    func acknowledge(seq: Int64, token: String) async throws {
        struct Request: Encodable { let seq: Int64 }
        try await sendWithoutResponse(
            method: "POST",
            path: "/v1/sync/ack",
            body: encode(Request(seq: seq)),
            token: token
        )
    }

    func devices(token: String) async throws -> DevicesResponse {
        try await send(DevicesResponse.self, method: "GET", path: "/v1/devices", token: token)
    }

    func revokeDevice(id: String, token: String) async throws {
        try await sendWithoutResponse(
            method: "POST",
            path: "/v1/devices/\(id)/revoke",
            token: token
        )
    }

    func webSocketRequest(token: String) throws -> URLRequest {
        var components = try components(path: "/v1/events/ws", queryItems: [])
        switch components.scheme?.lowercased() {
        case "https": components.scheme = "wss"
        case "http": components.scheme = "ws"
        default: throw APIClientError.invalidServerURL
        }
        guard let url = components.url else { throw APIClientError.invalidServerURL }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.setValue(Self.userAgent, forHTTPHeaderField: "User-Agent")
        return request
    }

    private func send<Response: Decodable>(
        _ type: Response.Type,
        method: String,
        path: String,
        queryItems: [URLQueryItem] = [],
        body: Data? = nil,
        token: String? = nil
    ) async throws -> Response {
        let data = try await sendData(
            method: method,
            path: path,
            queryItems: queryItems,
            body: body,
            token: token
        )
        do {
            return try Self.decoder.decode(type, from: data)
        } catch {
            throw APIClientError.invalidResponse
        }
    }

    private func sendWithoutResponse(
        method: String,
        path: String,
        queryItems: [URLQueryItem] = [],
        body: Data? = nil,
        token: String? = nil
    ) async throws {
        _ = try await sendData(
            method: method,
            path: path,
            queryItems: queryItems,
            body: body,
            token: token
        )
    }

    private func sendData(
        method: String,
        path: String,
        queryItems: [URLQueryItem],
        body: Data?,
        token: String?
    ) async throws -> Data {
        let components = try components(path: path, queryItems: queryItems)
        guard let url = components.url else { throw APIClientError.invalidServerURL }
        var request = URLRequest(url: url, timeoutInterval: 20)
        request.httpMethod = method
        request.httpBody = body
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue(Self.userAgent, forHTTPHeaderField: "User-Agent")
        if body != nil {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        if let token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIClientError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            if let envelope = try? Self.decoder.decode(APIErrorEnvelope.self, from: data) {
                throw APIClientError.server(
                    status: http.statusCode,
                    code: envelope.error.code,
                    message: envelope.error.message
                )
            }
            throw APIClientError.server(
                status: http.statusCode,
                code: "HTTP_\(http.statusCode)",
                message: "服务端请求失败（HTTP \(http.statusCode)）"
            )
        }
        return data
    }

    private func components(path: String, queryItems: [URLQueryItem]) throws -> URLComponents {
        guard var components = URLComponents(string: baseURL),
              let scheme = components.scheme?.lowercased(),
              ["http", "https"].contains(scheme),
              components.host != nil else {
            throw APIClientError.invalidServerURL
        }
        let prefix = components.path == "/" ? "" : components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        components.path = (prefix.isEmpty ? "" : "/\(prefix)") + path
        components.queryItems = queryItems.isEmpty ? nil : queryItems
        components.fragment = nil
        return components
    }

    private func encode<Value: Encodable>(_ value: Value) throws -> Data {
        try Self.encoder.encode(value)
    }

    private static let encoder: JSONEncoder = {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        return encoder
    }()

    private static let decoder: JSONDecoder = {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return decoder
    }()
}
