# Socrates for Android

The screens of `../app` and the engine of `../engine` in a [Capacitor](https://capacitorjs.com) shell. The engine is built as
`libsocratesengine.so` and runs as a child process of a foreground service; its database key is generated on the phone and wrapped by the
Android Keystore (`KeyHelper`). Nothing leaves the phone except what you send to your AI provider with your own key. Nothing is backed up
(the key would not open the data anywhere else).

Needs Go, Node 22, JDK 21, the Android SDK (platform 36, build-tools 36) and NDK 26 (`ANDROID_HOME` set).

```bash
cd mobile
scripts/build-engine.sh            # libsocratesengine.so for arm64-v8a (ABIS="arm64-v8a x86_64" adds the emulator)
scripts/build-apk.sh debug         # screens + sync + gradle; prints the APK path
```

`shim.js` is the phone's side of `window.socratesShell` (see `../app/app.js`). The content security policy of the page is widened by
`scripts/build-www.sh` to reach `http://127.0.0.1:*` only. Cleartext traffic is allowed to `127.0.0.1` only (`network_security_config.xml`).

Play Billing, the store listing and signing are not set up yet; the signing key must live outside the repository.
