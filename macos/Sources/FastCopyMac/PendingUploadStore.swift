import Foundation

struct PendingUploadStore {
    private let defaults = UserDefaults.standard
    private let key = "pendingUploads.v1"

    func load() -> [ClipUpload] {
        guard let data = defaults.data(forKey: key) else { return [] }
        return (try? JSONDecoder().decode([ClipUpload].self, from: data)) ?? []
    }

    func save(_ uploads: [ClipUpload]) {
        guard let data = try? JSONEncoder().encode(Array(uploads.suffix(100))) else { return }
        defaults.set(data, forKey: key)
    }

    func clear() {
        defaults.removeObject(forKey: key)
    }
}
