namespace FastCopy.Core;

public static class SyncTiming
{
    public static readonly TimeSpan ConnectedReconciliation = TimeSpan.FromMinutes(5);
    public static readonly TimeSpan DisconnectedReconciliation = TimeSpan.FromMinutes(1);
    private static readonly TimeSpan[] PendingRetryIntervals =
    [
        TimeSpan.FromSeconds(2),
        TimeSpan.FromSeconds(5),
        TimeSpan.FromSeconds(15),
        TimeSpan.FromSeconds(30),
        TimeSpan.FromSeconds(60)
    ];

    public static TimeSpan PendingRetry(int attempt)
    {
        var index = Math.Clamp(attempt, 0, PendingRetryIntervals.Length - 1);
        return PendingRetryIntervals[index];
    }
}
