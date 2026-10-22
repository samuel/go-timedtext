package lines

import (
	"reflect"
	"testing"
)

func TestAll(t *testing.T) {
	cases := map[string][]string{
		"":              nil,
		"a":             {"a"},
		"a\n":           {"a"},
		"a\nb":          {"a", "b"},
		"a\r\nb\r\n":    {"a", "b"},
		"a\rb\r":        {"a", "b"},
		"a\r\r\nb\n\nc": {"a", "", "b", "", "c"},
	}
	for in, exp := range cases {
		var got []string
		for n, line := range All(in) {
			if n != len(got)+1 {
				t.Errorf("All(%q) line number %d, expected %d", in, n, len(got)+1)
			}
			got = append(got, line)
		}
		if !reflect.DeepEqual(got, exp) {
			t.Errorf("All(%q) = %q, expected %q", in, got, exp)
		}
	}
	for range All("a\nb\nc") {
		break
	}
}
