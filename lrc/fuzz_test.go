package lrc

import (
	"bytes"
	"os"
	"testing"
)

// FuzzDecode checks that decoding never panics and that one decode and encode pass reaches a fixed point.
func FuzzDecode(f *testing.F) {
	for _, path := range []string{"testdata/example-in.lrc", "testdata/test.lrc"} {
		b, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(b))
	}
	f.Add("[ar:a]\n[00:01.00][00:02]M: <00:01.10>a <00:01.50>b\nc\n[00:03:50]F:\nd[00:04.00]e")
	f.Fuzz(func(t *testing.T, in string) {
		for _, dopt := range []DecodeOptions{{}, {Lenient: true, Voices: true}} {
			for _, eopt := range []EncodeOptions{{}, {HoursInTimecode: true, Multiline: true}} {
				first, ok := reencode(t, in, dopt, eopt)
				if !ok {
					continue
				}
				second, ok := reencode(t, first, dopt, eopt)
				if !ok {
					t.Fatalf("output with %+v %+v doesn't decode:\n%q", dopt, eopt, first)
				}
				if first != second {
					t.Fatalf("not a fixed point with %+v %+v:\n%q\n%q", dopt, eopt, first, second)
				}
			}
		}
	})
}

func reencode(t *testing.T, in string, dopt DecodeOptions, eopt EncodeOptions) (string, bool) {
	t.Helper()
	lyr, err := Decode(bytes.NewReader([]byte(in)), dopt)
	if err != nil {
		return "", false
	}
	var b bytes.Buffer
	if err := Encode(&b, lyr, eopt); err != nil {
		return "", false
	}
	return b.String(), true
}
