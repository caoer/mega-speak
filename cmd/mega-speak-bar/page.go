package main

import (
	"encoding/json"
	"strings"

	"github.com/caoer/mega-speak/internal/i18n"
)

// pageHTML is an app page with its lines in the app's language: @@STRINGS@@
// becomes i18n.Page(prefix) as JSON, which the page's script applies.
func pageHTML(page, prefix string) string {
	b, _ := json.Marshal(i18n.Page(prefix, i18n.Current()))
	return strings.Replace(page, "@@STRINGS@@", string(b), 1)
}
