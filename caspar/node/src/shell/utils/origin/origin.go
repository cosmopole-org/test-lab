package origin

import "strings"

func FindOrigin(id string) string {
	if id == "" {
		return ""
	} else {
		parts := strings.Split(id, "@")
		if len(parts) < 2 {
			return ""
		} else {
			return parts[len(parts)-1]
		}
	}
}

func LocalOnly(value string) string {
	if value == "global" {
		return ""
	}
	return value
}

func FindOriginLocal(id string) string {
	return LocalOnly(FindOrigin(id))
}
