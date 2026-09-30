package usersettings

import (
	"strings"
	"testing"
)

func TestTitleModelSettingValidation(t *testing.T) {
	if allowedKeys["chat.title_model"] != "follow" {
		t.Fatal("title model must default to follow")
	}
	for _, value := range []string{"follow", "gpt-4o-mini", "user_7_1_0123456789abcdef0123456789abcdef"} {
		if err := validateValue("chat.title_model", value); err != nil {
			t.Fatalf("valid model %q: %v", value, err)
		}
	}
	for _, value := range []string{"", " model ", strings.Repeat("x", 129)} {
		if err := validateValue("chat.title_model", value); err == nil {
			t.Fatalf("accepted invalid model %q", value)
		}
	}
}
