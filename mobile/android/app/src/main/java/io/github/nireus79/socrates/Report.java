package io.github.nireus79.socrates;

import android.content.Context;
import android.content.pm.PackageInfo;
import android.os.Build;
import android.os.PowerManager;

import java.io.File;
import java.io.RandomAccessFile;
import java.nio.charset.StandardCharsets;
import java.util.ArrayDeque;
import java.util.Date;
import java.util.Deque;
import java.util.regex.Pattern;

// The text behind "Report a problem": the phone, the app, whether the engine runs, and the engine's error lines.
// The engine keeps people's words out of its log (its privacy scrubber); the lines are cut short and key-like strings are masked
// anyway, and the person reads the whole text before choosing to share it.
final class Report {
    private static final Pattern ERROR_LINE = Pattern.compile("(?i)error|panic|fatal|fail|warning|unavailable");
    private static final Pattern SECRET = Pattern.compile("sk-[A-Za-z0-9_-]{8,}|\\b[0-9a-fA-F]{32,}\\b|Bearer\\s+\\S+");

    private Report() {}

    static String build(Context ctx) {
        StringBuilder b = new StringBuilder();
        b.append("Socrates report, ").append(new Date()).append('\n');
        try {
            PackageInfo p = ctx.getPackageManager().getPackageInfo(ctx.getPackageName(), 0);
            b.append("App ").append(ctx.getPackageName()).append(' ').append(p.versionName).append('\n');
        } catch (Exception ignored) {
        }
        b.append("Engine process ").append(EngineService.engineAlive() ? "running" : "not running (it stops by itself when the app is closed or idle)").append('\n');
        b.append("Phone ").append(Build.MANUFACTURER).append(' ').append(Build.MODEL).append(", Android ").append(Build.VERSION.RELEASE)
                .append(" (API ").append(Build.VERSION.SDK_INT).append("), ").append(String.join(",", Build.SUPPORTED_ABIS)).append('\n');
        PowerManager pm = (PowerManager) ctx.getSystemService(Context.POWER_SERVICE);
        if (pm != null) {
            b.append("Battery optimisation: ").append(pm.isIgnoringBatteryOptimizations(ctx.getPackageName()) ? "Socrates is exempt" : "Socrates is NOT exempt").append('\n');
        }
        b.append("\nEngine lines with an error or warning (newest last):\n");
        Deque<String> errs = tail(new File(ctx.getFilesDir(), "engine.log"), 40, ERROR_LINE);
        if (errs.isEmpty()) b.append("none\n");
        for (String l : errs) b.append(clean(l)).append('\n');
        return b.toString();
    }

    private static String clean(String line) {
        String s = SECRET.matcher(line).replaceAll("[hidden]");
        return s.length() > 200 ? s.substring(0, 200) + "..." : s;
    }

    // The last `max` lines of the file's final 256 KB that match `only`.
    private static Deque<String> tail(File f, int max, Pattern only) {
        Deque<String> out = new ArrayDeque<>();
        if (!f.exists()) return out;
        try (RandomAccessFile r = new RandomAccessFile(f, "r")) {
            long start = Math.max(0, r.length() - 256 * 1024);
            r.seek(start);
            byte[] buf = new byte[(int) (r.length() - start)];
            r.readFully(buf);
            for (String l : new String(buf, StandardCharsets.UTF_8).split("\n")) {
                if (l.isEmpty() || !only.matcher(l).find()) continue;
                out.addLast(l);
                if (out.size() > max) out.removeFirst();
            }
        } catch (Exception ignored) {
        }
        return out;
    }
}
