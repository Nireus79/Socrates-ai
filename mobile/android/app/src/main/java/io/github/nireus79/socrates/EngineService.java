package io.github.nireus79.socrates;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.IBinder;
import android.util.Log;

import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.File;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.Map;
import java.util.concurrent.TimeUnit;

// Owns the engine process. A foreground service keeps the app (and so the engine, its child) at foreground priority while a reply is
// being written, even with the screen off. The engine stops when its standard input closes (stopEngine), and by itself here when the app
// has been off the screen a while and the engine reports that nothing is happening.
public class EngineService extends Service {
    private static final String TAG = "Socrates";
    private static final long IDLE_STOP_MS = 3 * 60 * 1000;

    private static Process engine;
    private static OutputStream engineStdin;
    private static volatile int enginePort;
    private static volatile String engineToken;
    private static volatile boolean appVisible = false;
    private static volatile long lastVisible = System.currentTimeMillis();
    private static volatile int lastStartId;
    private static Thread watcher;

    static void appVisible(boolean visible) {
        appVisible = visible;
        lastVisible = System.currentTimeMillis();
    }

    // The port and token of the running engine, or null while it is not (yet) listening.
    static synchronized String[] connection() {
        if (engine == null || !engine.isAlive() || engineToken == null) return null;
        return new String[]{String.valueOf(enginePort), engineToken};
    }

    @Override
    public IBinder onBind(Intent i) {
        return null;
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        lastStartId = startId;
        lastVisible = System.currentTimeMillis();
        Notification.Builder nb = new Notification.Builder(this);
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationManager nm = getSystemService(NotificationManager.class);
            nm.createNotificationChannel(new NotificationChannel("engine", "Socrates in the background", NotificationManager.IMPORTANCE_LOW));
            nb = new Notification.Builder(this, "engine");
        }
        Notification n = nb.setContentTitle("Socrates is running")
                .setContentText("Lets replies finish while you are away; stops by itself when idle")
                .setSmallIcon(android.R.drawable.ic_popup_sync)
                .setOngoing(true)
                .build();
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(1, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
        } else {
            startForeground(1, n);
        }
        try {
            startEngine();
        } catch (Throwable t) {
            Log.e(TAG, "engine start failed", t);
        }
        return START_NOT_STICKY; // after being killed the person opens the app again, which starts the engine anew
    }

    private synchronized void startEngine() throws Exception {
        if (engine != null && engine.isAlive()) return;
        engineToken = null;
        File exe = new File(getApplicationInfo().nativeLibraryDir, "libsocratesengine.so");
        File dataDir = new File(getFilesDir(), "socrates");
        ProcessBuilder pb = new ProcessBuilder(exe.getAbsolutePath(), "--data-dir", dataDir.getAbsolutePath());
        Map<String, String> env = pb.environment();
        env.put("HOME", getFilesDir().getAbsolutePath());
        env.put("SOCRATES_DB_KEY", KeyHelper.loadOrCreateDbKey(this));
        env.put("SOCRATES_ALLOWED_ORIGINS", "https://localhost"); // the origin Capacitor gives the screens
        File engineLog = new File(getFilesDir(), "engine.log");
        trim(engineLog, 512 * 1024, 128 * 1024);
        pb.redirectError(ProcessBuilder.Redirect.appendTo(engineLog));
        final Process started = pb.start();
        engine = started;
        engineStdin = started.getOutputStream(); // kept open: closing it asks the engine to stop
        Thread reader = new Thread(() -> readHandshake(started));
        reader.setDaemon(true);
        reader.start();
        if (watcher == null) {
            watcher = new Thread(this::watch);
            watcher.setDaemon(true);
            watcher.start();
        }
    }

    // The engine prints one JSON line (port, token, version) on standard output when it listens; nothing else is expected there.
    private void readHandshake(Process p) {
        try {
            BufferedReader r = new BufferedReader(new InputStreamReader(p.getInputStream()));
            String line = r.readLine();
            if (line == null) return;
            JSONObject j = new JSONObject(line);
            synchronized (EngineService.class) {
                if (engine != p) return;
                enginePort = j.getInt("port");
                engineToken = j.getString("token");
            }
            while (r.readLine() != null) { /* drain */ }
        } catch (Throwable t) {
            Log.e(TAG, "no handshake from the engine", t);
        }
    }

    static synchronized void stopEngine() {
        Process p = engine;
        if (p == null) return;
        try {
            engineStdin.close(); // a clean stop: the engine closes its database
            if (!p.waitFor(12, TimeUnit.SECONDS)) p.destroyForcibly();
        } catch (Throwable t) {
            p.destroyForcibly();
        }
        engine = null;
        engineToken = null;
        watcher = null;
    }

    private void watch() {
        while (true) {
            try {
                Thread.sleep(60000);
            } catch (InterruptedException e) {
                return;
            }
            if (stopWhenIdle()) return;
        }
    }

    // Stops the engine and this service when the app has been off the screen for IDLE_STOP_MS and the engine reports that long with no
    // request in flight. Any doubt keeps it running. Opening the app again starts it anew (SocratesEnginePlugin.start).
    private boolean stopWhenIdle() {
        if (appVisible || System.currentTimeMillis() - lastVisible < IDLE_STOP_MS) return false;
        long idle = engineIdleSeconds();
        if (idle < IDLE_STOP_MS / 1000) return false;
        synchronized (EngineService.class) {
            if (appVisible) return false; // the person came back while we were deciding
        }
        stopEngine();
        stopForeground(true);
        stopSelf(lastStartId);
        return true;
    }

    private long engineIdleSeconds() {
        String[] c = connection();
        if (c == null) return -1;
        try {
            HttpURLConnection h = (HttpURLConnection) new URL("http://127.0.0.1:" + c[0] + "/internal/idle").openConnection();
            h.setConnectTimeout(3000);
            h.setReadTimeout(3000);
            h.setRequestProperty("X-Socrates-Token", c[1]);
            String body = new BufferedReader(new InputStreamReader(h.getInputStream())).readLine();
            return new JSONObject(body).getLong("idleSeconds");
        } catch (Throwable t) {
            return -1;
        }
    }

    // Android 15 and later limit a data-sync foreground service to about six hours a day and then call this: stop at once.
    @Override
    public void onTimeout(int startId, int fgsType) {
        stopEngine();
        stopForeground(true);
        stopSelf(startId);
    }

    @Override
    public void onDestroy() {
        super.onDestroy();
    }

    private static synchronized void trim(File f, long max, long keep) {
        if (!f.exists() || f.length() <= max) return;
        try (java.io.RandomAccessFile r = new java.io.RandomAccessFile(f, "r")) {
            r.seek(f.length() - keep);
            r.readLine();
            byte[] rest = new byte[(int) (r.length() - r.getFilePointer())];
            r.readFully(rest);
            try (java.io.FileOutputStream o = new java.io.FileOutputStream(f)) {
                o.write(rest);
            }
        } catch (Exception ignored) {
        }
    }
}
