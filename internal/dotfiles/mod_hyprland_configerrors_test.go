package dotfiles

import (
	"reflect"
	"testing"
)

func TestHyprConfigErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty string", raw: "", want: nil},
		{name: "empty json array", raw: "[]", want: nil},
		{name: "blank json array entry", raw: "[\"\"]", want: nil},
		{name: "whitespace json array entry", raw: "[\"   \"]", want: nil},
		{name: "json errors", raw: "[\"Invalid dispatcher foo\", \"bad rule\"]", want: []string{"Invalid dispatcher foo", "bad rule"}},
		{name: "json object error", raw: "{\"error\":\"Invalid dispatcher foo\"}", want: []string{"Invalid dispatcher foo"}},
		{name: "json object nested array", raw: "{\"errors\":[\"bad rule\", \"\"]}", want: []string{"bad rule"}},
		{name: "plain no errors text", raw: "no errors", want: nil},
		{name: "plain stderr", raw: "hyprctl unavailable", want: []string{"hyprctl unavailable"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hyprConfigErrorMessages(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("hyprConfigErrorMessages(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}
