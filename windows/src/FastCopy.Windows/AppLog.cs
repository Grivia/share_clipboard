using System.Text;

namespace FastCopy.Windows;

internal static class AppLog
{
    private const long MaxLogBytes = 512 * 1024;
    private static readonly object Gate = new();

    public static void Info(string message) => Write("INFO", message, null);

    public static void Error(string message, Exception exception) => Write("ERROR", message, exception);

    private static void Write(string level, string message, Exception? exception)
    {
        try
        {
            lock (Gate)
            {
                var directory = LocalStore.StorageDirectory;
                Directory.CreateDirectory(directory);
                var path = Path.Combine(directory, "fastcopy.log");
                if (File.Exists(path) && new FileInfo(path).Length > MaxLogBytes)
                {
                    File.Move(path, path + ".old", true);
                }

                var line = $"{DateTimeOffset.Now:O} [{level}] {message}";
                if (exception is not null)
                {
                    line += $" | {exception.GetType().Name}: {exception.Message}";
                }
                File.AppendAllText(path, line + Environment.NewLine, Encoding.UTF8);
            }
        }
        catch
        {
            // Logging must never interrupt clipboard synchronization.
        }
    }
}
