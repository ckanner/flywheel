package contract

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var investigationHeader = regexp.MustCompile("^[A-Za-z][A-Za-z0-9-]{0,63}$")

// ValidateInvestigationURL grants one exact destination, not a redirect chain.
// Query strings are permitted for explicit read-only queries; credentials must
// be supplied through named header environment variables, never URL userinfo.
func ValidateInvestigationURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(raw, "\r\n") {
		return fmt.Errorf("invalid investigation HTTP URL")
	}
	loopback := false
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return fmt.Errorf("investigation HTTP requires HTTPS or a loopback HTTP address")
	}
	return nil
}

func validateInvestigationAction(a InvestigationAction, paths map[string]bool, limits InvestigationLimits) error {
	invalid := func() error { return fmt.Errorf("ungranted or inconsistent investigation action: %s", a.ID) }
	if a.Kind == "run_command" {
		if a.Command == nil || a.HTTP != nil || a.Path != "" || a.Query != "" {
			return invalid()
		}
		c := a.Command
		if c.TimeoutMS > int64(limits.MaxSeconds)*1000 || strings.TrimSpace(c.Argv[0]) == "" {
			return invalid()
		}
		for _, input := range c.Inputs {
			if !paths[input] {
				return invalid()
			}
		}
		for _, arg := range c.Argv {
			if strings.ContainsRune(arg, 0) {
				return invalid()
			}
		}
		return nil
	}
	if a.Kind == "http_read" {
		if a.HTTP == nil || a.Command != nil || a.Path != "" || a.Query != "" {
			return invalid()
		}
		h := a.HTTP
		if err := ValidateInvestigationURL(h.URL); err != nil {
			return err
		}
		if h.TimeoutMS > int64(limits.MaxSeconds)*1000 || h.MaxBytes > limits.MaxOutputBytes {
			return invalid()
		}
		seen := map[string]bool{}
		for name := range h.HeadersEnv {
			if seen[strings.ToLower(name)] || !investigationHeader.MatchString(name) || strings.EqualFold(name, "Host") || strings.EqualFold(name, "Connection") || strings.EqualFold(name, "Content-Length") || strings.EqualFold(name, "Transfer-Encoding") || strings.EqualFold(name, "Proxy-Authorization") || strings.EqualFold(name, "Upgrade") || strings.EqualFold(name, "Accept-Encoding") {
				return invalid()
			}
			seen[strings.ToLower(name)] = true
		}
		return nil
	}
	if a.Command != nil || a.HTTP != nil || !paths[a.Path] || (a.Kind == "search_text") != (a.Query != "") {
		return invalid()
	}
	return nil
}
