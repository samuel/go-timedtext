package srt

import (
	"bytes"
	"testing"
)

// FuzzDecode checks that decoding never panics and that one decode and encode pass reaches a fixed point.
func FuzzDecode(f *testing.F) {
	f.Add(sample)
	f.Add("1\n00:00:01,000 --> 00:00:02,500 X1:1\n<i>a</i>\n\nb\n\n00:00:03,000 --> 00:00:04,000\nc\n")
	f.Fuzz(func(t *testing.T, in string) {
		first, ok := reencode(in)
		if !ok {
			return
		}
		second, ok := reencode(first)
		if !ok {
			t.Fatalf("output doesn't decode:\n%q", first)
		}
		if first != second {
			t.Fatalf("not a fixed point:\n%q\n%q", first, second)
		}
	})
}

func reencode(in string) (string, bool) {
	v, err := Decode(bytes.NewReader([]byte(in)), DecodeOptions{})
	if err != nil {
		return "", false
	}
	var b bytes.Buffer
	if err := Encode(&b, v); err != nil {
		return "", false
	}
	return b.String(), true
}
