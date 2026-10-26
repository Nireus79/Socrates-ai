package api

import "testing"

func TestThemeFollowsTheDeviceUntilChosenAndOnlyKnownValuesAreKept(t *testing.T) {
	a := newApp(t, newScript(t))
	a.setUp(t)
	if _, r := a.do(t, "GET", "/api/settings/theme", ""); r["theme"] != "auto" {
		t.Fatalf("default theme = %v", r["theme"])
	}
	for _, bad := range []string{`{"theme":"purple"}`, `{"theme":""}`, `{}`} {
		if code, _ := a.do(t, "PUT", "/api/settings/theme", bad); code != 400 {
			t.Fatalf("%s: code = %d", bad, code)
		}
	}
	if code, _ := a.do(t, "PUT", "/api/settings/theme", `{"theme":"dark"}`); code != 200 {
		t.Fatalf("code = %d", code)
	}
	if _, r := a.do(t, "GET", "/api/settings/theme", ""); r["theme"] != "dark" {
		t.Fatalf("theme = %v", r["theme"])
	}
	if code, _ := a.do(t, "PUT", "/api/settings/theme", `{"theme":"auto"}`); code != 200 {
		t.Fatalf("code = %d", code)
	}
	if _, r := a.do(t, "GET", "/api/settings/theme", ""); r["theme"] != "auto" {
		t.Fatalf("theme = %v", r["theme"])
	}
}
