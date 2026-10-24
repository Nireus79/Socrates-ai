package shell

import "testing"

func TestOnlyTheKeyPagesOfTheTwoProvidersMayBeOpened(t *testing.T) {
	for _, ok := range []string{
		"https://platform.claude.com/settings/keys",
		"https://platform.openai.com/api-keys",
		"https://platform.openai.com/settings/organization/billing/overview",
		"https://platform.claude.com/",
	} {
		if !AllowedLink(ok) {
			t.Errorf("refused %s", ok)
		}
	}
	for _, bad := range []string{
		"", "platform.claude.com", "http://platform.claude.com/", "https://evil.example/", "https://platform.claude.com.evil.example/",
		"https://user:pw@platform.claude.com/", "https://platform.claude.com:8443/", "file:///etc/passwd", "javascript:alert(1)",
		"https://claude.com/", "ftp://platform.openai.com/", "https://platform.openai.com@evil.example/", "https://evil.example/?u=https://platform.claude.com/",
	} {
		if AllowedLink(bad) {
			t.Errorf("allowed %q", bad)
		}
	}
}
