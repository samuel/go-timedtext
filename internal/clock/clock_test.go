package clock

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := map[string]time.Duration{
		"00:01.5":       1500 * time.Millisecond,
		"00:00:01,250":  1250 * time.Millisecond,
		"01:02:03.004":  time.Hour + 2*time.Minute + 3*time.Second + 4*time.Millisecond,
		"100:00:00":     100 * time.Hour,
		"00:9223372035": 9223372035 * time.Second,
	}
	for s, exp := range cases {
		d, err := Parse(s)
		if err != nil {
			t.Errorf("Parse(%q) failed: %s", s, err)
		} else if d != exp {
			t.Errorf("Parse(%q) = %s, expected %s", s, d, exp)
		}
	}
	for _, s := range []string{"", "1", "1:2:3:4", "a:00", "-1:00", "00:00.x", "99999999999999:00:00", "00:9223372036", "00:9223372094", "1:00:9223372035"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) succeeded, expected an error", s)
		}
	}
}

func TestFormat(t *testing.T) {
	d := time.Minute + 2*time.Second + 3456*time.Microsecond
	if s := Format(d, ',', false); s != "00:01:02,003" {
		t.Errorf("Format = %q", s)
	}
	if s := Format(d, '.', true); s != "01:02.003" {
		t.Errorf("Format short = %q", s)
	}
	if s := Format(2*time.Hour, '.', true); s != "02:00:00.000" {
		t.Errorf("Format short with hours = %q", s)
	}
}

func TestDuration(t *testing.T) {
	if d, err := Duration([]string{"215"}, "5"); err != nil || d != 215500*time.Millisecond {
		t.Errorf("Duration with one field = %s, %v", d, err)
	}
	for _, fields := range [][]string{nil, {"1", "2", "3", "4"}} {
		if _, err := Duration(fields, ""); err == nil {
			t.Errorf("Duration(%q) succeeded, expected an error", fields)
		}
	}
}

func TestParseTiming(t *testing.T) {
	cases := []struct {
		in         string
		start, end time.Duration
		settings   string
	}{
		{"00:01.000 --> 00:02.000", time.Second, 2 * time.Second, ""},
		{"00:00:01,000-->00:00:02,000", time.Second, 2 * time.Second, ""},
		{"00:01.000 --> 00:02.000 align:start  line:0", time.Second, 2 * time.Second, "align:start  line:0"},
		{"00:01.000 --> 00:02.000\talign:start", time.Second, 2 * time.Second, "align:start"},
	}
	for _, c := range cases {
		start, end, settings, err := ParseTiming(c.in)
		if err != nil {
			t.Errorf("ParseTiming(%q) failed: %s", c.in, err)
		} else if start != c.start || end != c.end || settings != c.settings {
			t.Errorf("ParseTiming(%q) = %s, %s, %q", c.in, start, end, settings)
		}
	}
	for _, s := range []string{"00:01.000", "00:01.000 -->", "x --> 00:01.000"} {
		if _, _, _, err := ParseTiming(s); err == nil {
			t.Errorf("ParseTiming(%q) succeeded, expected an error", s)
		}
	}
}
