package ui

import "strings"

// projectAbbreviationLimit is how wide a project tag may be before it is
// abbreviated. A card is about thirty columns, so a long name would otherwise
// take a third of the row.
const projectAbbreviationLimit = 8

// ShortProjectName abbreviates a project name for a card or a list row.
//
// A hyphenated or spaced name becomes its initials ("my-side-project" → "msp");
// a single long word is truncated ("a-very-long-name" → truncated). Names within
// the limit are returned unchanged, so most projects read normally.
func ShortProjectName(project string) string {
	trimmed := strings.TrimSpace(project)
	if trimmed == "" || len([]rune(trimmed)) <= projectAbbreviationLimit {
		return trimmed
	}

	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '/' || r == '.'
	})
	if len(parts) > 1 {
		var initials strings.Builder
		for _, part := range parts {
			runes := []rune(part)
			if len(runes) == 0 {
				continue
			}
			initials.WriteRune(runes[0])
		}
		if initials.Len() > 1 {
			return initials.String()
		}
	}

	return string([]rune(trimmed)[:projectAbbreviationLimit])
}
