package main

import (
	_ "embed"
	"encoding/json"
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
