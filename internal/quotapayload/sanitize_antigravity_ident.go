package quotapayload

import (
	"regexp"
	"strings"

	"quota-activation/internal/detector"
	"quota-activation/internal/state"
)

func sanitizeModelKey(rawKey string, safeProvider string) (key string, exactClean bool, ok bool) {
	trimmed := strings.TrimSpace(rawKey)
	if trimmed == "" {
		return "", false, false
	}
	if group, found := detector.InferModelGroup(trimmed); found {
		if isCleanModelIdentifier(trimmed) {
			return trimmed, true, true
		}
		return canonicalGroupModel(group), false, true
	}
	if safeProvider != "" {
		if group, found := detector.InferModelGroup(safeProvider); found {
			return canonicalGroupModel(group), false, true
		}
	}
	return "", false, false
}

const maxModelIdentifierLen = 128

var (
	credPrefixCI = regexp.MustCompile(`(?i)^(?:ya29\.|sk-(?:proj|ant|svcacct|admin|or|live|test|none)-|(?:sk|rk|pk)_(?:live|test)_|rk-(?:live|test)-|whsec_|gh[pousr]_[a-z0-9]{6,}|github_pat_[a-z0-9_]{6,}|gl(?:pat|oas|dt)-[a-z0-9_-]{6,}|xox[bpoas]-[a-z0-9-]{6,}|xapp-[a-z0-9-]{6,}|(?:gsk_|pplx-|npm_|pypi-)[a-z0-9_-]{8,}|sess-[a-z0-9_-]{10,}|xai-[a-z0-9]{16,}|basic_[a-z0-9_-]{8,}|hf_[a-z0-9]{8,})`)
	credPrefixCS = regexp.MustCompile(`^(?:AIza[A-Za-z0-9_-]{6,}|(?:AKIA|ASIA|AIDA|AROA)[A-Z0-9]{12,}(?:$|\.)|eyJ(?:hbG|0eX|[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]{4,}))`)
	googleAIzaRe = regexp.MustCompile(`AIza[A-Za-z0-9_-]{6,}`)
	credWordRe   = regexp.MustCompile(`(?i)(?:^|[/:\-_.])jwt(?:$|[/:\-_.])|api[_-]?key|password|passwd|private[_-]?key|privatekey|credential`)
	modelWords   = []string{"gemini", "claude", "gpt", "openai", "argon"}
	pathWords    = []string{"anthropic", "google", "antigravity", "codex", "vertex", "models", "publishers", "locations", "projects", "flash", "sonnet", "opus", "haiku", "thinking", "preview"}
)

func isCleanModelIdentifier(value string) bool {
	if len(value) == 0 || len(value) > maxModelIdentifierLen || state.Redact(value) != value {
		return false
	}
	if !hasCleanModelDelimiters(value) || credWordRe.MatchString(value) {
		return false
	}
	segments := strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == ':' })
	for i, seg := range segments {
		if hasCredentialPrefixAtBoundary(seg) || hasOpaqueSecretSegment(seg) {
			return false
		}
		if strings.EqualFold(seg, "basic") && i+1 < len(segments) && len(segments[i+1]) >= 8 && !hasWordMatch(segments[i+1], modelWords, false) {
			return false
		}
	}
	return true
}

func hasCleanModelDelimiters(value string) bool {
	isAlnum := func(b byte) bool {
		return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
	}
	if !isAlnum(value[0]) || !isAlnum(value[len(value)-1]) {
		return false
	}
	var prev rune
	var prevSymbol bool
	for i, r := range value {
		if r < 128 && isAlnum(byte(r)) {
			prev, prevSymbol = r, false
			continue
		}
		if r != '-' && r != '_' && r != '.' && r != ':' && r != '/' {
			return false
		}
		if i > 0 && prevSymbol && (r != prev || (r != '-' && r != '_')) {
			return false
		}
		prev, prevSymbol = r, true
	}
	return true
}

func hasCredentialPrefixAtBoundary(seg string) bool {
	if googleAIzaRe.MatchString(seg) {
		return true
	}
	for i := 0; i < len(seg); i++ {
		if i > 0 && seg[i-1] != '-' && seg[i-1] != '_' && seg[i-1] != '.' {
			continue
		}
		sub := seg[i:]
		if credPrefixCS.MatchString(sub) || isGenericSKSecret(sub) {
			return true
		}
		if credPrefixCI.MatchString(sub) {
			if len(sub) >= 3 && strings.EqualFold(sub[:3], "hf_") && hasWordMatch(sub[3:], modelWords, true) {
				continue
			}
			return true
		}
	}
	return false
}

func isGenericSKSecret(sub string) bool {
	if len(sub) <= 3 || !strings.EqualFold(sub[:3], "sk-") {
		return false
	}
	rest, _, _ := strings.Cut(sub[3:], ".")
	if rest == "" || hasWordMatch(rest, modelWords, true) {
		return false
	}
	compact := strings.NewReplacer("-", "", "_", "", ".", "").Replace(rest)
	classes, _, hasDigit := inspectRun(compact)
	return hasDigit || classes >= 2 || len(compact) >= 8
}

func hasOpaqueSecretSegment(seg string) bool {
	mixedSubTokens := 0
	tokens := strings.FieldsFunc(seg, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for _, tok := range tokens {
		classes, isHex, _ := inspectRun(tok)
		if len(tok) >= 24 && isHex {
			return true
		}
		if hasWordMatch(tok, modelWords, false) {
			continue
		}
		if (len(tok) >= 16 && classes == 3) || (len(tok) >= 24 && classes >= 2) {
			return true
		}
		if len(tok) >= 8 && classes == 3 {
			mixedSubTokens++
		}
	}
	if mixedSubTokens >= 2 || hasWordMatch(seg, modelWords, false) || hasWordMatch(seg, pathWords, false) {
		return mixedSubTokens >= 2
	}
	compact := strings.NewReplacer("-", "", "_", "", ".", "").Replace(seg)
	classes, isHex, _ := inspectRun(compact)
	return (len(compact) >= 24 && isHex) || (len(compact) >= 16 && classes == 3) || (len(compact) >= 28 && classes >= 2)
}

func inspectRun(s string) (classes int, isHex bool, hasDigit bool) {
	var hasLower, hasUpper, hasNum int
	isHex = s != ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
			hasLower, isHex = 1, isHex && c <= 'f'
		case c >= 'A' && c <= 'Z':
			hasUpper, isHex = 1, isHex && c <= 'F'
		case c >= '0' && c <= '9':
			hasNum, hasDigit = 1, true
		default:
			isHex = false
		}
	}
	return hasLower + hasUpper + hasNum, isHex, hasDigit
}

func hasWordMatch(s string, words []string, prefixOnly bool) bool {
	lower := strings.ToLower(s)
	for _, w := range words {
		if (prefixOnly && strings.HasPrefix(lower, w)) || (!prefixOnly && strings.Contains(lower, w)) {
			return true
		}
	}
	return false
}

func sanitizeModelProvider(raw any) string {
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return canonicalGroupTokens(text, true)
}

func sanitizeGroupDisplayName(rawDisplay any, rawDesc any) string {
	disp, _ := rawDisplay.(string)
	desc, _ := rawDesc.(string)
	return canonicalGroupTokens(disp+" "+desc, false)
}

func canonicalGroupTokens(text string, includeAnthropic bool) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ""
	}
	hasArgon := hasGemini4ArgonToken(lower)
	hasClaude := strings.Contains(lower, "claude")
	hasGPT := strings.Contains(lower, "gpt")
	hasOpenAI := strings.Contains(lower, "openai")
	hasGemini := strings.Contains(lower, "gemini") && !hasClaude && !hasGPT && !hasArgon

	tokens := make([]string, 0, 5)
	if hasClaude {
		tokens = append(tokens, "claude")
	}
	if hasGPT {
		tokens = append(tokens, "gpt")
	}
	if hasOpenAI {
		tokens = append(tokens, "openai")
	}
	if hasArgon {
		tokens = append(tokens, "gemini-4-argon")
	}
	if hasGemini {
		tokens = append(tokens, "gemini")
	}
	if includeAnthropic && strings.Contains(lower, "anthropic") {
		tokens = append(tokens, "anthropic")
	}
	return strings.Join(tokens, " ")
}

func canonicalGroupModel(group detector.ModelGroup) string {
	switch group {
	case detector.ModelGroupGemini:
		return "gemini"
	case detector.ModelGroupClaudeGPT:
		return "claude-gpt"
	case detector.ModelGroupGemini4Argon:
		return "gemini-4-argon"
	case detector.ModelGroupNone:
		return ""
	default:
		return ""
	}
}

func hasGemini4ArgonToken(text string) bool {
	normalized := strings.ToLower(text)
	normalized = strings.ReplaceAll(normalized, "-", " ")
	normalized = strings.ReplaceAll(normalized, "_", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")
	return strings.Contains(normalized, "gemini 4 argon")
}
