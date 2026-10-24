package shell

import "net/url"

// AllowedLink says whether the window may open an address in the person's browser: only the sites where a key is made, over https,
// with no user name and no port. The page asks only for these, and this check does not trust the page.
func AllowedLink(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	return u.Hostname() == "platform.claude.com" || u.Hostname() == "platform.openai.com"
}
