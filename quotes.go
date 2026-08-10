package main

import (
	_ "embed"
	"encoding/json"
	"time"
)

// quote is one entry in the "quote of the day" rotation shown under the hub logo.
type quote struct {
	Text   string `json:"text"`
	Author string `json:"author"`
}

//go:embed quotes.json
var quotesJSON []byte

// loadQuotes parses the embedded quote set. A parse failure (should not happen for an
// embedded, build-time-checked asset) yields a nil slice — the quote line simply doesn't
// render, mirroring the tolerant missing/corrupt → zero value pattern used by loadUserConfig.
func loadQuotes() []quote {
	var qs []quote
	if err := json.Unmarshal(quotesJSON, &qs); err != nil {
		return nil
	}
	return qs
}

const quoteDateLayout = "2006-01-02"

// quoteIndexForToday resolves which of the n quotes to show today. today is injected for
// testability. hasConfigDir distinguishes "no persistable config" (os.UserConfigDir() failed)
// from "config dir exists but firstLaunch not yet recorded" — only the latter requests a save.
func quoteIndexForToday(uc userConfig, today time.Time, hasConfigDir bool, n int) (index int, needsSave bool) {
	if !hasConfigDir {
		return today.YearDay() % n, false
	}
	first, err := time.Parse(quoteDateLayout, uc.FirstLaunch)
	if uc.FirstLaunch == "" || err != nil {
		return 0, true
	}
	days := int(today.Sub(first).Hours() / 24)
	if days < 0 {
		days = -days
	}
	return days % n, false
}
