package security

import (
	"crypto/subtle"
	"regexp"
	"strings"
	"unicode"
)

const (
	MaxInputLength     = 100_000
	MaxOutputLength    = 64 * 1024
	MaxMemoryEntrySize = 4096
	MaxMemoryStoreSize = 10 * 1024 * 1024
	MaxToolResultSize  = 32 * 1024
)

var (
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,})`),
		regexp.MustCompile(`(?i)(Bearer\s+[a-zA-Z0-9\-._~+/]+=*)`),
		regexp.MustCompile(`(?i)(api[_-]?key\s*[=:]\s*\S{10,})`),
		regexp.MustCompile(`(?i)(token\s*[=:]\s*\S{10,})`),
		regexp.MustCompile(`(?i)(password\s*[=:]\s*\S{4,})`),
		regexp.MustCompile(`(?i)(secret\s*[=:]\s*\S{10,})`),
		regexp.MustCompile(`(?i)(ghp_[a-zA-Z0-9]{36})`),
		regexp.MustCompile(`(?i)(xox[baprs]-[a-zA-Z0-9\-]+)`),
	}

	exfilPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(curl|wget)\s+.*https?://`),
		regexp.MustCompile(`(?i)base64\s+-d`),
		regexp.MustCompile(`(?i)(/etc/passwd|/etc/shadow)`),
		regexp.MustCompile(`(?i)(\.env\b|\.git/credentials)`),
		regexp.MustCompile(`(?i)(send\s+to\s+https?://|exfil|exfiltrate)`),
		regexp.MustCompile(`(?i)(ignore\s+(all|previous)\s+(instructions|rules))`),
		regexp.MustCompile(`(?i)(you\s+are\s+now|disregard\s+prior|system\s*:\s*)`),
	}

	sensitivePaths = []string{
		"/etc/passwd", "/etc/shadow", "/etc/sudoers",
		"/proc/", "/sys/", "/boot/",
		"/var/run/docker.sock",
		"/root/.ssh/", "/root/.gnupg/",
	}

	controlCharFilter = func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		if r >= 0xD800 && r <= 0xDFFF {
			return -1
		}
		return r
	}
)

func SanitizeUnicode(input string) string {
	return strings.Map(controlCharFilter, input)
}

func Truncate(input string, maxLen int) string {
	if len(input) <= maxLen {
		return input
	}
	return input[:maxLen]
}

func ValidateLength(input string, maxLen int) error {
	if len(input) > maxLen {
		return &ValidationError{
			Field:   "input",
			Message: "input exceeds maximum length",
			Limit:   maxLen,
			Actual:  len(input),
		}
	}
	return nil
}

func ContainsSecrets(input string) bool {
	for _, pat := range secretPatterns {
		if pat.MatchString(input) {
			return true
		}
	}
	return false
}

func RedactSecrets(input string) string {
	result := input
	for _, pat := range secretPatterns {
		result = pat.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}

func DetectExfiltration(input string) bool {
	for _, pat := range exfilPatterns {
		if pat.MatchString(input) {
			return true
		}
	}
	return false
}

func DetectPromptInjection(input string) bool {
	injectionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(ignore\s+(all|previous|above))`),
		regexp.MustCompile(`(?i)(disregard\s+(all|previous|instructions))`),
		regexp.MustCompile(`(?i)(you\s+are\s+now\s+)`),
		regexp.MustCompile(`(?i)(system\s*:\s*|<system>)`),
		regexp.MustCompile(`(?i)(new\s+instructions?\s*:)`),
		regexp.MustCompile(`(?i)(override\s+(your|the)\s+(instructions|system\s*prompt))`),
		regexp.MustCompile(`(?i)(print\s+your\s+(system\s*prompt|instructions|config))`),
		regexp.MustCompile(`(?i)(what\s+are\s+your\s+(instructions|rules))`),
	}
	for _, pat := range injectionPatterns {
		if pat.MatchString(input) {
			return true
		}
	}
	return false
}

func IsSensitivePath(path string) bool {
	lower := strings.ToLower(path)
	for _, sp := range sensitivePaths {
		if strings.HasPrefix(lower, strings.ToLower(sp)) {
			return true
		}
	}
	return false
}

func HasPathTraversal(path string) bool {
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	if strings.Contains(path, "..\\") {
		return true
	}
	return false
}

func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

type ValidationError struct {
	Field   string
	Message string
	Limit   int
	Actual  int
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field + " " + e.Message
}

type InputValidator struct {
	MaxInputLength  int
	MaxOutputLength int
}

func NewInputValidator() *InputValidator {
	return &InputValidator{
		MaxInputLength:  MaxInputLength,
		MaxOutputLength: MaxOutputLength,
	}
}

func (v *InputValidator) ValidateInput(input string) error {
	if err := ValidateLength(input, v.MaxInputLength); err != nil {
		return err
	}
	if DetectPromptInjection(input) {
		return &ValidationError{
			Field:   "input",
			Message: "potential prompt injection detected",
		}
	}
	return nil
}

func (v *InputValidator) ValidateOutput(output string) error {
	if err := ValidateLength(output, v.MaxOutputLength); err != nil {
		return err
	}
	if DetectExfiltration(output) {
		return &ValidationError{
			Field:   "output",
			Message: "potential data exfiltration pattern detected",
		}
	}
	return nil
}

func (v *InputValidator) ValidateMemoryEntry(content string) error {
	if err := ValidateLength(content, MaxMemoryEntrySize); err != nil {
		return err
	}
	return nil
}
