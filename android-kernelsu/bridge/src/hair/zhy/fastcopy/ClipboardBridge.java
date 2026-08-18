package hair.zhy.fastcopy;

import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.os.Process;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.PrintWriter;
import java.lang.reflect.Constructor;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Base64;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

public final class ClipboardBridge {
    private static final long POLL_INTERVAL_MS = 700;
    private static final int MAX_SECURITY_FAILURES = 3;
    private static final int SHELL_UID = 2000;
    private static final int SYSTEM_UID = 1000;
    private static final String SHELL_PACKAGE = "com.android.shell";
    private static final String SYSTEM_PACKAGE = "android";
    private static final String BACKGROUND_CLIPBOARD_PERMISSION =
            "android.permission.READ_CLIPBOARD_IN_BACKGROUND";

    private final ClipboardAccess access;
    private final ClipboardManager clipboard;
    private final Handler handler;
    private final PrintWriter protocol;
    private String lastText;
    private boolean initialized;
    private boolean readFailureLogged;
    private int consecutiveSecurityFailures;

    private ClipboardBridge(ClipboardAccess access, Handler handler) {
        this.access = access;
        this.clipboard = access.manager;
        this.handler = handler;
        this.protocol = new PrintWriter(System.out, true);
    }

    public static void main(String[] args) {
        try {
            Looper.prepareMainLooper();
            Handler handler = new Handler(Looper.getMainLooper());
            Context systemContext = createSystemContext();
            ClipboardAccess access = createClipboardAccess(systemContext, handler);
            ClipboardBridge bridge = new ClipboardBridge(access, handler);
            bridge.start();
            Looper.loop();
        } catch (Throwable error) {
            System.err.println("fatal: " + describe(error));
            error.printStackTrace(System.err);
            System.exit(1);
        }
    }

    private void start() {
        System.err.println(
                "clipboard identity: sdk=" + Build.VERSION.SDK_INT
                        + " uid=" + Process.myUid()
                        + " package=" + access.packageName
                        + " op_package=" + access.context.getOpPackageName()
                        + " strategy=" + access.strategy
                        + " background_permission=granted");
        clipboard.addPrimaryClipChangedListener(new ClipboardManager.OnPrimaryClipChangedListener() {
            @Override
            public void onPrimaryClipChanged() {
                poll();
            }
        });
        Thread commands = new Thread(new Runnable() {
            @Override
            public void run() {
                readCommands();
            }
        }, "fastcopy-commands");
        commands.setDaemon(true);
        commands.start();
        protocol.println("READY");
        poll();
        schedulePoll();
    }

    private void periodicPoll() {
        poll();
        schedulePoll();
    }

    private void schedulePoll() {
        handler.postDelayed(new Runnable() {
            @Override
            public void run() {
                periodicPoll();
            }
        }, POLL_INTERVAL_MS);
    }

    private void poll() {
        try {
            String text = readText();
            readFailureLogged = false;
            consecutiveSecurityFailures = 0;
            if (text == null || text.equals(lastText)) {
                initialized = true;
                return;
            }
            lastText = text;
            emit(initialized ? "CLIP" : "INIT", text);
            initialized = true;
        } catch (SecurityException error) {
            handleSecurityFailure("read", error);
        } catch (Throwable error) {
            if (!readFailureLogged) {
                System.err.println("clipboard read failed: " + describe(error));
                readFailureLogged = true;
            }
        }
    }

    private String readText() {
        if (!clipboard.hasPrimaryClip()) {
            return null;
        }
        ClipData data = clipboard.getPrimaryClip();
        if (data == null || data.getItemCount() == 0) {
            return null;
        }
        CharSequence text = data.getItemAt(0).getText();
        return text == null ? null : text.toString();
    }

    private void readCommands() {
        try (BufferedReader reader = new BufferedReader(
                new InputStreamReader(System.in, StandardCharsets.UTF_8))) {
            String line;
            while ((line = reader.readLine()) != null) {
                if (!line.startsWith("SET\t")) {
                    continue;
                }
                String encoded = line.substring(4);
                String text = new String(Base64.getDecoder().decode(encoded), StandardCharsets.UTF_8);
                handler.post(new Runnable() {
                    @Override
                    public void run() {
                        setText(text);
                    }
                });
            }
        } catch (Throwable error) {
            System.err.println("command reader stopped: " + describe(error));
            System.exit(2);
        }
        System.exit(0);
    }

    private void setText(String text) {
        try {
            clipboard.setPrimaryClip(ClipData.newPlainText("FastCopy", text));
            lastText = text;
            initialized = true;
            String applied = readText();
            if (!text.equals(applied)) {
                protocol.println("SET_RETRY\tclipboard_not_observable");
                return;
            }
            consecutiveSecurityFailures = 0;
            protocol.println("SET_OK");
        } catch (SecurityException error) {
            protocol.println("SET_RETRY\tsecurity_denied");
            handleSecurityFailure("write", error);
        } catch (Throwable error) {
            System.err.println("clipboard write failed: " + describe(error));
            protocol.println("SET_RETRY\twrite_failed");
        }
    }

    private void handleSecurityFailure(String operation, SecurityException error) {
        consecutiveSecurityFailures++;
        if (consecutiveSecurityFailures == 1) {
            System.err.println("clipboard " + operation + " denied: " + describe(error));
        }
        if (consecutiveSecurityFailures >= MAX_SECURITY_FAILURES) {
            System.err.println("clipboard identity rejected repeatedly; trying another launcher");
            System.exit(4);
        }
    }

    private void emit(String type, String text) {
        String encoded = Base64.getEncoder().encodeToString(text.getBytes(StandardCharsets.UTF_8));
        protocol.println(type + "\t" + encoded);
    }

    private static Context createSystemContext() throws Exception {
        Class<?> activityThreadClass = Class.forName("android.app.ActivityThread");
        Object activityThread = null;
        try {
            Method current = activityThreadClass.getDeclaredMethod("currentActivityThread");
            current.setAccessible(true);
            activityThread = current.invoke(null);
        } catch (NoSuchMethodException ignored) {
            // Older or vendor-modified frameworks can omit this helper.
        }
        if (activityThread == null) {
            Method systemMain = activityThreadClass.getDeclaredMethod("systemMain");
            systemMain.setAccessible(true);
            activityThread = systemMain.invoke(null);
        }
        Method getSystemContext = activityThreadClass.getDeclaredMethod("getSystemContext");
        getSystemContext.setAccessible(true);
        Context context = (Context) getSystemContext.invoke(activityThread);
        if (context == null) {
            throw new IllegalStateException("ActivityThread returned no system context");
        }
        return context;
    }

    private static ClipboardAccess createClipboardAccess(Context systemContext, Handler handler)
            throws Exception {
        int uid = Process.myUid();
        PackageManager packageManager = systemContext.getPackageManager();
        List<String> packages = packagesForUid(packageManager, uid);
        Throwable lastError = null;

        for (String packageName : packages) {
            if (packageManager.checkPermission(BACKGROUND_CLIPBOARD_PERMISSION, packageName)
                    != PackageManager.PERMISSION_GRANTED) {
                System.err.println(
                        "clipboard package rejected: " + packageName
                                + " lacks " + BACKGROUND_CLIPBOARD_PERMISSION);
                continue;
            }
            for (ContextCandidate candidate : contextCandidates(systemContext, packageName)) {
                try {
                    ClipboardManager manager = createDirectClipboardManager(
                            candidate.context,
                            handler);
                    probeClipboard(manager);
                    return new ClipboardAccess(
                            candidate.context,
                            manager,
                            packageName,
                            candidate.strategy + "+direct");
                } catch (Throwable error) {
                    lastError = unwrap(error);
                    System.err.println(
                            "clipboard strategy rejected: " + candidate.strategy
                                    + "+direct: " + describe(lastError));
                }

                try {
                    ClipboardManager manager = (ClipboardManager) candidate.context
                            .getSystemService(Context.CLIPBOARD_SERVICE);
                    if (manager == null) {
                        throw new IllegalStateException("ClipboardManager is unavailable");
                    }
                    probeClipboard(manager);
                    return new ClipboardAccess(
                            candidate.context,
                            manager,
                            packageName,
                            candidate.strategy + "+service");
                } catch (Throwable error) {
                    lastError = unwrap(error);
                    System.err.println(
                            "clipboard strategy rejected: " + candidate.strategy
                                    + "+service: " + describe(lastError));
                }
            }
        }

        String suffix = lastError == null ? "" : ": " + describe(lastError);
        throw new IllegalStateException(
                "no compatible clipboard identity for uid " + uid + suffix,
                lastError);
    }

    private static List<String> packagesForUid(PackageManager packageManager, int uid) {
        Set<String> ordered = new LinkedHashSet<String>();
        if (uid == SHELL_UID) {
            ordered.add(SHELL_PACKAGE);
        } else if (uid == SYSTEM_UID) {
            ordered.add(SYSTEM_PACKAGE);
        }
        String[] discovered = packageManager.getPackagesForUid(uid);
        if (discovered != null) {
            for (String packageName : discovered) {
                if (packageName != null && !packageName.isEmpty()) {
                    ordered.add(packageName);
                }
            }
        }

        List<String> verified = new ArrayList<String>();
        for (String packageName : ordered) {
            try {
                if (packageManager.getPackageUid(packageName, 0) == uid) {
                    verified.add(packageName);
                }
            } catch (PackageManager.NameNotFoundException ignored) {
                // Keep looking for another package owned by this UID.
            }
        }
        if (verified.isEmpty()) {
            throw new IllegalStateException("no installed package belongs to uid " + uid);
        }
        return verified;
    }

    private static List<ContextCandidate> contextCandidates(
            Context systemContext,
            String packageName) {
        List<ContextCandidate> candidates = new ArrayList<ContextCandidate>();
        try {
            Context packageContext = systemContext.createPackageContext(
                    packageName,
                    Context.CONTEXT_IGNORE_SECURITY);
            candidates.add(new ContextCandidate(
                    new IdentityContext(packageContext, packageName),
                    "package-forced"));
            candidates.add(new ContextCandidate(packageContext, "package-native"));
        } catch (PackageManager.NameNotFoundException error) {
            System.err.println(
                    "package context unavailable for " + packageName + ": " + describe(error));
        }
        candidates.add(new ContextCandidate(
                new IdentityContext(systemContext, packageName),
                "system-forced"));
        return candidates;
    }

    private static ClipboardManager createDirectClipboardManager(Context context, Handler handler)
            throws Exception {
        Constructor<ClipboardManager> constructor = ClipboardManager.class.getDeclaredConstructor(
                Context.class,
                Handler.class);
        constructor.setAccessible(true);
        return constructor.newInstance(context, handler);
    }

    private static void probeClipboard(ClipboardManager manager) {
        manager.hasPrimaryClip();
    }

    private static Throwable unwrap(Throwable error) {
        Throwable current = error;
        while (current instanceof InvocationTargetException
                && ((InvocationTargetException) current).getCause() != null) {
            current = ((InvocationTargetException) current).getCause();
        }
        return current;
    }

    private static String describe(Throwable error) {
        Throwable unwrapped = unwrap(error);
        String message = unwrapped.getMessage();
        if (message == null || message.isEmpty()) {
            return unwrapped.getClass().getName();
        }
        return unwrapped.getClass().getName() + ": " + message;
    }

    private static final class ClipboardAccess {
        private final Context context;
        private final ClipboardManager manager;
        private final String packageName;
        private final String strategy;

        private ClipboardAccess(
                Context context,
                ClipboardManager manager,
                String packageName,
                String strategy) {
            this.context = context;
            this.manager = manager;
            this.packageName = packageName;
            this.strategy = strategy;
        }
    }

    private static final class ContextCandidate {
        private final Context context;
        private final String strategy;

        private ContextCandidate(Context context, String strategy) {
            this.context = context;
            this.strategy = strategy;
        }
    }

    private static final class IdentityContext extends ContextWrapper {
        private final String packageName;

        private IdentityContext(Context base, String packageName) {
            super(base);
            this.packageName = packageName;
        }

        @Override
        public String getPackageName() {
            return packageName;
        }

        @Override
        public String getOpPackageName() {
            return packageName;
        }
    }
}
