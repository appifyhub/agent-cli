package config

import (
	"net"
	"net/url"
	"strings"
)

// APIURL is the backend address: THE_AGENT_CLI_API_URL, else the config file, else the build default,
// without trailing slashes. It does not validate the address, so informational commands never fail on it;
// callers that send a request pass the result to ValidateAPIURL first.
func APIURL(getenv func(string) string, f File, buildDefault string) (string, error) {
	for _, candidate := range []string{getenv(EnvAPIURL), f.APIURL, buildDefault} {
		if candidate != "" {
			return strings.TrimRight(candidate, "/"), nil
		}
	}
	return "", ErrNoAPIURL
}

// ValidateAPIURL accepts https URLs, and http only for loopback hosts, and rejects embedded credentials,
// a query string and a fragment. The returned *InvalidURLError never contains the credentials.
func ValidateAPIURL(raw string) error {
	reject := func(problem URLProblem) error {
		return &InvalidURLError{URL: withoutCredentials(raw), Problem: problem}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return reject(URLMalformed)
	}
	switch {
	case u.User != nil:
		return reject(URLCredentials)
	case u.RawQuery != "" || u.Fragment != "" || u.ForceQuery:
		return reject(URLQueryOrFragment)
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && isLoopback(u.Hostname()):
		return nil
	}
	return reject(URLInsecure)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// withoutCredentials drops anything before the "@" of the authority part, whether or not the URL parses.
func withoutCredentials(raw string) string {
	scheme, rest, found := strings.Cut(raw, "://")
	if !found {
		return raw
	}
	authority, path := rest, ""
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		authority, path = rest[:i], rest[i:]
	}
	if i := strings.LastIndexByte(authority, '@'); i >= 0 {
		authority = authority[i+1:]
	}
	return scheme + "://" + authority + path
}
