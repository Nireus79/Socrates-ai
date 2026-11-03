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

// AllowedSponsorLink says whether the window may open the link of a card (the card's messages come from the project's own data file and
// are checked by the engine and the page as well): any https address with a host, no user name, no port, of reasonable length.
func AllowedSponsorLink(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 300 && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Port() == ""
}
