// Package clock parses and formats the hh:mm:ss.mmm timestamps used by subtitle formats.
package clock

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Parse parses [hh:]mm:ss with an optional fraction after a '.' or ','. The fraction is scaled by its
// digit count.
func Parse(s string) (time.Duration, error) {
	clock, frac, _ := strings.Cut(strings.ReplaceAll(s, ",", "."), ".")
	fields := strings.Split(clock, ":")
	if len(fields) < 2 || len(fields) > 3 {
		return 0, fmt.Errorf("invalid timestamp %q", s)
	}
	d, err := Duration(fields, frac)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp %q: %w", s, err)
	}
	return d, nil
}

// Duration converts one to three colon-separated fields (hours, minutes, seconds), the last of which is
// seconds, and a decimal fraction of a second.
func Duration(fields []string, frac string) (time.Duration, error) {
	if len(fields) == 0 || len(fields) > 3 {
		return 0, fmt.Errorf("%d fields, expected 1 to 3", len(fields))
	}
	// Leaves room for the fraction so the result can't overflow.
	const maxSeconds = math.MaxInt64/int64(time.Second) - 1
	var secs int64
	for _, f := range fields {
		n, err := parseDigits(f)
		if err != nil {
			return 0, err
		}
		if n > maxSeconds || secs > (maxSeconds-n)/60 {
			return 0, errors.New("value too large")
		}
		secs = secs*60 + n
	}
	d := time.Duration(secs) * time.Second
	if frac != "" {
		// Nanosecond precision is the most a time.Duration can hold.
		if len(frac) > 9 {
			frac = frac[:9]
		}
		n, err := parseDigits(frac)
		if err != nil {
			return 0, err
		}
		for range 9 - len(frac) {
			n *= 10
		}
		d += time.Duration(n)
	}
	return d, nil
}

func parseDigits(s string) (int64, error) {
	if s == "" || strings.TrimLeft(s, "0123456789") != "" {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	return strconv.ParseInt(s, 10, 64)
}

// Format formats a non-negative duration as hh:mm:ss followed by sep and milliseconds, rounded to the
// nearest millisecond. When short is set and the duration is under an hour, the hours are left out.
func Format(d time.Duration, sep byte, short bool) string {
	d = d.Round(time.Millisecond)
	h := d / time.Hour
	m := d % time.Hour / time.Minute
	s := d % time.Minute / time.Second
	ms := d % time.Second / time.Millisecond
	if short && h == 0 {
		return fmt.Sprintf("%02d:%02d%c%03d", m, s, sep, ms)
	}
	return fmt.Sprintf("%02d:%02d:%02d%c%03d", h, m, s, sep, ms)
}

// ParseTiming parses a cue timing line such as "00:01.000 --> 00:02.000 align:start" into its start and
// end times and any settings after the end time.
func ParseTiming(line string) (start, end time.Duration, settings string, err error) {
	s, rest, ok := strings.Cut(line, "-->")
	if !ok {
		return 0, 0, "", errors.New("missing -->")
	}
	e := strings.TrimLeft(rest, " \t")
	if i := strings.IndexAny(e, " \t"); i >= 0 {
		e, settings = e[:i], strings.TrimSpace(e[i:])
	}
	if start, err = Parse(strings.TrimSpace(s)); err != nil {
		return 0, 0, "", err
	}
	if end, err = Parse(e); err != nil {
		return 0, 0, "", err
	}
	return start, end, settings, nil
}
