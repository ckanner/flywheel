package config

// ClaudeCodeProCredentialAllowed matches the public machine API's explicit
// credential overrides. An unrecognized key must not silently disappear when
// the plugin filters the worker environment.
func ClaudeCodeProCredentialAllowed(name string) bool {
	switch name {
	case "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN":
		return true
	default:
		return false
	}
}
