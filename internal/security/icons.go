package security

// validIcons is the whitelist of icon keys the server accepts.
// Matches the files in internal/server/web/icons/*.svg.
var validIcons = map[string]struct{}{
	"document": {}, "folder": {}, "star": {}, "bookmark": {},
	"tag": {}, "image": {}, "link": {}, "code": {}, "book": {},
	"idea": {}, "note": {}, "calendar": {}, "task": {}, "archive": {},
	"heart": {}, "flag": {},
}

// ValidateIcon returns true if icon is empty (no icon) or is a known icon key.
func ValidateIcon(icon string) bool {
	if icon == "" {
		return true
	}
	_, ok := validIcons[icon]
	return ok
}
