package streamers

import (
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// GenerateID returns a random alphanumeric identifier for new streamer records.
func GenerateID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

// NormaliseAlias collapses a streamer alias into its lowercase alphanumeric form.
// It is used for validation and duplicate detection.
func NormaliseAlias(alias string) string {
	var builder strings.Builder
	for _, r := range alias {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
		}
	}
	return builder.String()
}
