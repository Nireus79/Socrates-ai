package io.github.nireus79.socrates;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;

import androidx.activity.result.ActivityResult;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.ActivityCallback;
import com.getcapacitor.annotation.CapacitorPlugin;

import java.io.OutputStream;
import java.nio.charset.StandardCharsets;

// The phone's replacement for the desktop shell: starts the engine (in the foreground service) and hands the screens its port and token;
// saves an export where the person chooses.
@CapacitorPlugin(name = "SocratesEngine")
public class SocratesEnginePlugin extends Plugin {
    private String pendingSave;

    @PluginMethod
    public void start(PluginCall call) {
        Context ctx = getContext();
        Intent i = new Intent(ctx, EngineService.class);
        if (Build.VERSION.SDK_INT >= 26) ctx.startForegroundService(i); else ctx.startService(i);
        // wait for the engine's handshake without blocking the bridge thread
        new Thread(() -> {
            long end = System.currentTimeMillis() + 30000;
            while (System.currentTimeMillis() < end) {
                String[] c = EngineService.connection();
                if (c != null) {
                    JSObject r = new JSObject();
                    r.put("port", Integer.parseInt(c[0]));
                    r.put("token", c[1]);
                    call.resolve(r);
                    return;
                }
                try {
                    Thread.sleep(100);
                } catch (InterruptedException e) {
                    break;
                }
            }
            call.reject("The engine did not start.");
        }).start();
    }

    // The key help: open a provider's page in the phone's browser. Only the two sites where a key is made, over https, are opened; the
    // page's own check is not trusted.
    @PluginMethod
    public void openLink(PluginCall call) {
        Uri uri = Uri.parse(call.getString("url", ""));
        String host = uri.getHost();
        boolean allowed = "https".equals(uri.getScheme()) && uri.getUserInfo() == null && uri.getPort() == -1
                && ("platform.claude.com".equals(host) || "platform.openai.com".equals(host));
        if (!allowed) {
            call.reject("That address is not one the app opens.");
            return;
        }
        try {
            Intent i = new Intent(Intent.ACTION_VIEW, uri);
            i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            getContext().startActivity(i);
            call.resolve();
        } catch (Exception e) {
            call.reject("No browser could be opened.");
        }
    }

    // "Export": Android's "save as" picker, so the person chooses where the file goes.
    @PluginMethod
    public void saveFile(PluginCall call) {
        pendingSave = call.getString("text", "");
        Intent i = new Intent(Intent.ACTION_CREATE_DOCUMENT);
        i.addCategory(Intent.CATEGORY_OPENABLE);
        i.setType("application/json");
        i.putExtra(Intent.EXTRA_TITLE, call.getString("name", "socrates-export.json"));
        startActivityForResult(call, i, "saveFileResult");
    }

    @ActivityCallback
    private void saveFileResult(PluginCall call, ActivityResult result) {
        if (call == null) return;
        String text = pendingSave;
        pendingSave = null;
        Uri uri = result.getResultCode() == Activity.RESULT_OK && result.getData() != null ? result.getData().getData() : null;
        JSObject r = new JSObject();
        if (uri == null || text == null) {
            r.put("saved", false); // the person cancelled
            call.resolve(r);
            return;
        }
        try (OutputStream o = getContext().getContentResolver().openOutputStream(uri, "wt")) {
            o.write(text.getBytes(StandardCharsets.UTF_8));
            r.put("saved", true);
            call.resolve(r);
        } catch (Exception e) {
            call.reject("The file could not be saved.");
        }
    }
}
