package textenc

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// Normalize converts common Chinese source encodings to valid UTF-8 before
// content enters storage, prompts, callbacks or exports. JSON is UTF-8 by
// contract, but integrations frequently forward GBK/GB18030 text from legacy
// archives or OCR services.
func Normalize(value string) string {
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "\uFEFF")
	value = strings.ReplaceAll(value, "\x00", "")
	if utf8.ValidString(value) {
		if strings.ContainsRune(value, '\uFFFD') {
			if decoded, ok := decodeBytes([]byte(value)); ok {
				return decoded
			}
			return value
		}
		if repaired, ok := repairLatin1Mojibake(value); ok {
			return repaired
		}
		return value
	}
	if decoded, ok := decodeBytes([]byte(value)); ok {
		return decoded
	}
	return strings.ToValidUTF8(value, "")
}

func repairLatin1Mojibake(value string) (string, bool) {
	runes := []rune(value)
	if len(runes) < 4 || containsHan(runes) {
		return "", false
	}
	latin1Count := 0
	raw := make([]byte, 0, len(runes))
	for _, r := range runes {
		if r > 0xFF {
			return "", false
		}
		if r >= 0xA0 {
			latin1Count++
		}
		raw = append(raw, byte(r))
	}
	if latin1Count < 4 || latin1Count*2 < len(runes) {
		return "", false
	}
	decoded, ok := decodeBytes(raw)
	if !ok || countHan([]rune(decoded)) < 2 {
		return "", false
	}
	return decoded, true
}

func decodeBytes(raw []byte) (string, bool) {
	encodings := []encoding.Encoding{
		simplifiedchinese.GB18030,
		traditionalchinese.Big5,
	}
	best := ""
	bestScore := -1
	for _, enc := range encodings {
		decoded, _, err := transform.Bytes(enc.NewDecoder(), raw)
		if err != nil || !utf8.Valid(decoded) {
			continue
		}
		text := string(decoded)
		if strings.ContainsRune(text, '\uFFFD') {
			continue
		}
		score := textScore([]rune(text))
		if score > bestScore {
			best, bestScore = text, score
		}
	}
	if bestScore < 2 {
		return "", false
	}
	return best, true
}

func textScore(runes []rune) int {
	han := 0
	score := 0
	for _, r := range runes {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
			score += 3
		case strings.ContainsRune("，。；：！？、（）《》【】“”‘’", r):
			score += 2
		case r == '\n' || r == '\r' || r == '\t':
			score++
		case unicode.IsControl(r):
			score -= 6
		case r == '\uFFFD':
			score -= 8
		}
	}
	if han == 0 {
		return score
	}
	return score + 2
}

func containsHan(runes []rune) bool {
	return countHan(runes) > 0
}

func countHan(runes []rune) int {
	count := 0
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) {
			count++
		}
	}
	return count
}
