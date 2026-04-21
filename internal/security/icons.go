package security

import "regexp"

var iconPattern = regexp.MustCompile(`^bx[sl]?-[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// ValidateIcon returns true if icon is empty or is a valid Boxicons class name.
func ValidateIcon(icon string) bool {
	if icon == "" {
		return true
	}
	return len(icon) <= 60 && iconPattern.MatchString(icon)
}
