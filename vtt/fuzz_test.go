package vtt

import (
	"bytes"
	"testing"
)

// FuzzDecode checks that decoding never panics and that one decode and encode pass reaches a fixed point.
func FuzzDecode(f *testing.F) {
	f.Add(sample)
	f.Add("WEBVTT\nLanguage: en\n\nNOTE x\n\nid\n00:01.000 --> 00:02.000\talign:start\n<v Bob>hi <00:01.500>there &amp;\n")
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
