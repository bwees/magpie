package parse

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"magpie/internal/domain"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// iso6391Length is the length of an ISO 639-1 code, the form domain.Language holds.
const iso6391Length = 2

// bibliographicCodes are the ISO 639-2/B codes that differ from the terminology codes x/text knows.
var bibliographicCodes = map[string]domain.Language{
	"alb": "sq", "arm": "hy", "baq": "eu", "bur": "my", "chi": "zh", "cze": "cs", "dut": "nl",
	"fre": "fr", "geo": "ka", "ger": "de", "gre": "el", "ice": "is", "mac": "mk", "mao": "mi",
	"may": "ms", "per": "fa", "rum": "ro", "slo": "sk", "tib": "bo", "wel": "cy",
}

// englishLanguageNames maps lower-case English language names ("english", "german") to ISO 639-1 codes.
var englishLanguageNames = func() map[string]domain.Language {
	names := map[string]domain.Language{}
	namer := display.English.Languages()
	for first := 'a'; first <= 'z'; first++ {
		for second := 'a'; second <= 'z'; second++ {
			base, err := language.ParseBase(string([]rune{first, second}))
			if err != nil {
				continue
			}
			if name := namer.Name(base); name != "" {
				names[strings.ToLower(name)] = domain.Language(base.String())
			}
		}
	}
	return names
}()

// LanguageCode reads an ISO 639-1 code, an ISO 639-2 code (terminology or bibliographic) or an English
// language name, and returns the ISO 639-1 code. Languages without an ISO 639-1 code aren't recognised.
func LanguageCode(s string) (domain.Language, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if code, ok := bibliographicCodes[s]; ok {
		return code, true
	}
	if code, ok := englishLanguageNames[s]; ok {
		return code, true
	}
	base, err := language.ParseBase(s)
	if err != nil || len(base.String()) != iso6391Length {
		return "", false
	}
	return domain.Language(base.String()), true
}

// LanguageName is a language's English name, or its code when x/text doesn't know it.
func LanguageName(code domain.Language) string {
	base, err := language.ParseBase(string(code))
	if err != nil {
		return string(code)
	}
	if name := display.English.Languages().Name(base); name != "" {
		return name
	}
	return string(code)
}

// SubtitleLanguage is what a subtitle file's name says about it.
type SubtitleLanguage struct {
	Language domain.Language
	Forced   bool
	SDH      bool
}

var (
	forcedWords = []string{"forced", "foreign"}
	sdhWords    = []string{"sdh", "cc", "hi"}
)

// ParseSubtitleLanguage reads the language from the end of a subtitle file name, such as "Movie.en.srt",
// "Movie.eng.forced.srt", "Movie.pt-BR.srt" or "2_English.srt". ok is false when the name ends in
// anything else.
func ParseSubtitleLanguage(path string) (SubtitleLanguage, bool) {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	words := strings.FieldsFunc(stem, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == ' ' || r == '[' || r == ']' || r == '(' || r == ')'
	})
	var found SubtitleLanguage
	for len(words) > 0 {
		last := words[len(words)-1]
		rest := words[:len(words)-1]
		word := strings.ToLower(last)
		switch {
		case slices.Contains(forcedWords, word):
			found.Forced = true
		case slices.Contains(sdhWords, word) && endsInLanguage(rest):
			found.SDH = true
		case isRegion(last) && endsInLanguage(rest):
		default:
			code, ok := LanguageCode(last)
			found.Language = code
			return found, ok
		}
		words = rest
	}
	return SubtitleLanguage{}, false
}

// endsInLanguage reports whether the last word is a language, so a flag or region before it can be skipped.
func endsInLanguage(words []string) bool {
	if len(words) == 0 {
		return false
	}
	_, ok := LanguageCode(words[len(words)-1])
	return ok
}

// isRegion is an upper-case region subtag such as the "BR" of "pt-BR".
func isRegion(word string) bool {
	if len(word) != iso6391Length {
		return false
	}
	for _, r := range word {
		if !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}
