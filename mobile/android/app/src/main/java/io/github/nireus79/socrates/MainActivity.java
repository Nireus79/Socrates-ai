package io.github.nireus79.socrates;

import android.Manifest;
import android.os.Build;
import android.os.Bundle;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle savedInstanceState) {
        registerPlugin(SocratesEnginePlugin.class);
        super.onCreate(savedInstanceState);
        if (Build.VERSION.SDK_INT >= 33) requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 1);
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

    // Closing the app with the back button or from the recent list ends the engine cleanly; the service would otherwise outlive the screens.
    @Override
    public void onDestroy() {
        if (isFinishing()) EngineService.stopEngine();
        super.onDestroy();
    }
}
