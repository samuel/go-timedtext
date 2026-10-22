package ttml

import (
	"bytes"
	"testing"
)

// FuzzDecode checks that decoding never panics and that one decode and encode pass reaches a fixed point.
func FuzzDecode(f *testing.F) {
	f.Add(sample)
	f.Add(`<tt xmlns="http://www.w3.org/ns/ttml"><body><div><p begin="1s" end="2s"><span begin="1s" end="1.5s">hi</span> there<br/>x</p></div></body></tt>`)
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
