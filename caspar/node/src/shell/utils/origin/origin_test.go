package origin

import "testing"

func TestFindOrigin(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "empty", id: "", want: ""},
		{name: "without-separator", id: "abc", want: ""},
		{name: "single-origin", id: "user@fed", want: "fed"},
		{name: "multiple-at", id: "user@store@fed", want: "fed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FindOrigin(tt.id); got != tt.want {
				t.Fatalf("FindOrigin(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestLocalOnlyAndFindOriginLocal(t *testing.T) {
	if got := LocalOnly("global"); got != "" {
		t.Fatalf("LocalOnly(global) = %q, want empty", got)
	}
	if got := LocalOnly("fed"); got != "fed" {
		t.Fatalf("LocalOnly(fed) = %q, want fed", got)
	}
	if got := FindOriginLocal("user@global"); got != "" {
		t.Fatalf("FindOriginLocal(user@global) = %q, want empty", got)
	}
	if got := FindOriginLocal("user@local-fed"); got != "local-fed" {
		t.Fatalf("FindOriginLocal(user@local-fed) = %q, want local-fed", got)
	}
}
