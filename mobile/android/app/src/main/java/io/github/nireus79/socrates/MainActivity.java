package io.github.nireus79.socrates;

import android.Manifest;
import android.os.Build;
import android.os.Bundle;
import android.webkit.WebView;

import androidx.activity.OnBackPressedCallback;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle savedInstanceState) {
        registerPlugin(SocratesEnginePlugin.class);
        super.onCreate(savedInstanceState);
        if (Build.VERSION.SDK_INT >= 33) requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 1);
        // Capacitor itself does not handle the Back key. The screens keep their own steps (app.js), because a WebView ignores history steps
        // that were made without a touch and would swallow quick presses. So Back asks the page: it answers true when it went back one
        // screen, and false at the first screen, where the app leaves as usual.
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                WebView w = getBridge() != null ? getBridge().getWebView() : null;
                if (w == null) {
                    leave();
                    return;
                }
                w.evaluateJavascript("(function(){return !!(window.socratesBack && window.socratesBack());})()", answer -> {
                    if (!"true".equals(answer)) leave();
                });
            }

            private void leave() {
                // Back at the first screen closes the app. Older Android versions would only move it to the background, where the engine
                // would keep running until the idle stop; finishing ends it (onDestroy asks the service to stop).
                finish();
            }
        });
    }

    @Override
    public void onStart() {
        super.onStart();
        EngineService.appVisible(true);
    }

    @Override
    public void onStop() {
        EngineService.appVisible(false);
        super.onStop();
    }

    // Closing the app with the back button or from the recent list ends the engine cleanly once nothing is being written, and the service with it.
    @Override
    public void onDestroy() {
        if (isFinishing()) EngineService.closeWhenDone();
        super.onDestroy();
    }
}
