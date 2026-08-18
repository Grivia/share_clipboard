namespace FastCopy.Windows;

internal static class Program
{
    [STAThread]
    private static void Main()
    {
        try
        {
            Run();
        }
        catch (Exception exception)
        {
            ReportFatalStartupException(exception);
        }
    }

    private static void Run()
    {
        using var mutex = new Mutex(true, "Local\\hair.zhy.FastCopy.Windows", out var ownsMutex);
        if (!ownsMutex)
        {
            MessageBox.Show(
                "FastCopy 已经在运行，请查看任务栏通知区域。",
                "FastCopy",
                MessageBoxButtons.OK,
                MessageBoxIcon.Information);
            return;
        }

        ApplicationConfiguration.Initialize();
        Application.SetUnhandledExceptionMode(UnhandledExceptionMode.CatchException);
        Application.ThreadException += (_, eventArgs) =>
        {
            AppLog.Error("Unhandled UI exception", eventArgs.Exception);
            MessageBox.Show(
                "FastCopy 遇到错误，详情已写入本地日志。",
                "FastCopy",
                MessageBoxButtons.OK,
                MessageBoxIcon.Error);
        };
        AppDomain.CurrentDomain.UnhandledException += (_, eventArgs) =>
        {
            if (eventArgs.ExceptionObject is Exception exception)
            {
                AppLog.Error("Unhandled background exception", exception);
            }
        };
        SynchronizationContext.SetSynchronizationContext(new WindowsFormsSynchronizationContext());

        using var context = new TrayApplicationContext();
        Application.Run(context);
    }

    private static void ReportFatalStartupException(Exception exception)
    {
        var fallbackPath = Path.Combine(Path.GetTempPath(), "FastCopy-startup-error.txt");
        try
        {
            AppLog.Error("Fatal startup exception", exception);
            File.WriteAllText(
                fallbackPath,
                $"{DateTimeOffset.Now:O}{Environment.NewLine}{exception}");
        }
        catch
        {
        }

        try
        {
            MessageBox.Show(
                $"FastCopy 启动失败。错误信息已写入：{Environment.NewLine}{fallbackPath}"
                    + $"{Environment.NewLine}{Environment.NewLine}{exception.Message}",
                "FastCopy",
                MessageBoxButtons.OK,
                MessageBoxIcon.Error);
        }
        catch
        {
        }
    }
}
