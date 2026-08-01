package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/client9/gospell"
	"okashi/internal/textarea"
)

//go:embed assets/fr.aff
var affData string

//go:embed assets/fr.dic
var dicData string

var (
	spellOnce sync.Once
	speller   *gospell.GoSpell
)

var (
	suggestMu    sync.Mutex
	suggestCache = map[string][]string{}
)

func loadSpeller() {
	speller, _ = gospell.NewGoSpellReader(strings.NewReader(affData), strings.NewReader(dicData))
	loadPersonalDictionary()
}

var misspellStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Underline(true)

// spellOK reports whether word is spelled correctly (gospell handles case,
// contractions, and possessives).
func spellOK(word string) bool {
	spellOnce.Do(loadSpeller)
	if inPersonalDict(word) {
		return true
	}
	if speller == nil {
		return true
	}
	return speller.Spell(word)
}

// spellSuggest returns up to limit correction candidates, best first (memoized —
// gospell's Suggest is heavier than Spell and this is called per frame by the
// passive status hint).
func spellSuggest(word string, limit int) []string {
	spellOnce.Do(loadSpeller)
	if speller == nil {
		return nil
	}
	key := word + "\x00" + strconv.Itoa(limit)
	suggestMu.Lock()
	if v, ok := suggestCache[key]; ok {
		suggestMu.Unlock()
		return v
	}
	suggestMu.Unlock()

	ss, err := speller.Suggest(word, limit)
	var out []string
	if err == nil {
		out = make([]string, 0, len(ss))
		for _, s := range ss {
			out = append(out, s.Word)
		}
	}
	suggestMu.Lock()
	if len(suggestCache) > 4096 {
		suggestCache = map[string][]string{}
	}
	suggestCache[key] = out
	suggestMu.Unlock()
	return out
}

// wordSpans returns [start,end) rune ranges of word tokens (letters + apostrophe).
func wordSpans(line string) [][2]int {
	var spans [][2]int
	runes := []rune(line)
	i := 0
	for i < len(runes) {
		if unicode.IsLetter(runes[i]) {
			j := i + 1
			for j < len(runes) && (unicode.IsLetter(runes[j]) || runes[j] == '\'') {
				j++
			}
			spans = append(spans, [2]int{i, j})
			i = j
		} else {
			i++
		}
	}
	return spans
}

func isAllCaps(s string) bool { return s == strings.ToUpper(s) && s != strings.ToLower(s) }

func hasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// spellDecorator flags misspelled words (red underline). All-caps acronyms and
// tokens with digits are skipped.
func spellDecorator(line string) []textarea.Decoration {
	var decos []textarea.Decoration
	runes := []rune(line)
	for _, s := range wordSpans(line) {
		word := strings.Trim(string(runes[s[0]:s[1]]), "'")
		if word == "" || isAllCaps(word) || hasDigit(word) {
			continue
		}
		if !spellOK(word) {
			decos = append(decos, textarea.Decoration{Start: s[0], End: s[1], Style: misspellStyle})
		}
	}
	return decos
}

// dictItem is the sentinel "suggestion" that adds the word under the cursor to the personal
// dictionary; it rides in the spell suggestion list so it's discoverable without a new key.
const dictItem = "＋ add to dict"

var (
	personalMu     sync.Mutex
	personalWords  = map[string]bool{}
	personalLoaded bool
)

func dictionaryPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "dictionary.txt")
}

// loadPersonalDictionary loads the global personal word list (once).
func loadPersonalDictionary() {
	personalMu.Lock()
	defer personalMu.Unlock()
	if personalLoaded {
		return
	}
	personalLoaded = true
	p := dictionaryPath()
	if p == "" {
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		w := strings.TrimSpace(line)
		if w == "" || strings.HasPrefix(w, "#") {
			continue
		}
		personalWords[strings.ToLower(w)] = true
	}
}

func inPersonalDict(word string) bool {
	personalMu.Lock()
	defer personalMu.Unlock()
	return personalWords[strings.ToLower(word)]
}

// addToDictionary adds word to the personal dictionary (memory + ~/.config/okashi/dictionary.txt).
// Returns true if it was newly added. Skips empty/numeric/all-caps and surrounding punctuation.
func addToDictionary(word string) bool {
	word = strings.Trim(strings.TrimSpace(word), "'\"().,;:!?“”’")
	if word == "" || hasDigit(word) || isAllCaps(word) {
		return false
	}
	personalMu.Lock()
	if personalWords[strings.ToLower(word)] {
		personalMu.Unlock()
		return false
	}
	personalWords[strings.ToLower(word)] = true
	personalMu.Unlock()

	if p := dictionaryPath(); p != "" {
		os.MkdirAll(filepath.Dir(p), 0o755)
		out := ""
		if existing, err := os.ReadFile(p); err == nil {
			out = string(existing)
			if out != "" && !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
		}
		out += word + "\n"
		atomicWrite(p, []byte(out), 0o644)
	}
	suggestMu.Lock()
	suggestCache = map[string][]string{}
	suggestMu.Unlock()
	return true
}
