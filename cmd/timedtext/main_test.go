package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvert(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "song.lrc")
	if err := os.WriteFile(in, []byte("[ti:Song]\n[00:01.00]a\n[00:02.50]b\n[00:04.00]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srtPath := filepath.Join(dir, "song.srt")
	if err := convert(in, srtPath, options{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(srtPath)
	if err != nil {
		t.Fatal(err)
	}
	exp := "1\n00:00:01,000 --> 00:00:02,500\na\n\n2\n00:00:02,500 --> 00:00:04,000\nb\n\n"
	if string(b) != exp {
		t.Errorf("SRT output = %q, expected %q", b, exp)
	}

	back := filepath.Join(dir, "back.lrc")
	for _, path := range []string{filepath.Join(dir, "song.vtt"), filepath.Join(dir, "song.ttml")} {
		if err := convert(srtPath, path, options{}); err != nil {
			t.Fatal(err)
		}
		if err := convert(path, back, options{}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(back)
		if err != nil {
			t.Fatal(err)
		}
		if exp := "[00:01.00]a\n[00:02.50]b\n[00:04.00]\n"; string(b) != exp {
			t.Errorf("LRC from %s = %q, expected %q", filepath.Ext(path), b, exp)
		}
	}

	if err := convert(in, filepath.Join(dir, "song.txt"), options{}); err == nil {
		t.Error("expected error for unknown output format")
	}
}

func TestConvertStdio(t *testing.T) {
	var out bytes.Buffer
	stdin, stdout = strings.NewReader("[00:01.00]a\n[00:02.00]b\n"), &out
	t.Cleanup(func() { stdin, stdout = os.Stdin, os.Stdout })
	if err := convert("-", "-", options{from: "lrc", to: "vtt"}); err != nil {
		t.Fatal(err)
	}
	exp := "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\na\n\n00:00:02.000 --> 00:00:07.000\nb\n\n"
	if out.String() != exp {
		t.Errorf("output = %q, expected %q", out.String(), exp)
	}
	for _, c := range []struct {
		in, out string
		opt     options
	}{
		{"-", "-", options{to: "vtt"}},
		{"-", "-", options{from: "lrc"}},
		{"-", "-", options{from: "txt", to: "vtt"}},
	} {
		if err := convert(c.in, c.out, c.opt); err == nil {
			t.Errorf("convert(%q, %q, %+v) succeeded, expected an error", c.in, c.out, c.opt)
		}
	}
}

func TestConvertFormatFlags(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "song.txt")
	if err := os.WriteFile(in, []byte("[00:01.00]a[99999999999:00.00]b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "song.out")
	if err := os.WriteFile(out, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := convert(in, out, options{from: "lrc", to: "srt"}); err == nil {
		t.Fatal("expected error for an invalid timecode")
	}
	if b, err := os.ReadFile(out); err != nil || string(b) != "old" {
		t.Fatalf("failed conversion changed the output: %q, %v", b, err)
	}
	if err := convert(in, out, options{from: "lrc", to: "srt", lenient: true}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if exp := "1\n00:00:01,000 --> 00:00:06,000\na[99999999999:00.00]b\n\n"; string(b) != exp {
		t.Errorf("output = %q, expected %q", b, exp)
	}
}

// FuzzConvert checks that any decoded input converts to every format without error.
func FuzzConvert(f *testing.F) {
	f.Add("lrc", "[ti:T]\n[00:01.00][00:02]M: <00:01.10>a <00:01.50>b\nc\n[00:03:50]F:\nd[00:04.00]e\n[00:05.00]")
	f.Add("srt", "1\n00:00:01,000 --> 00:00:02,500\n<i>a</i>\n\n2\n00:00:02,000 --> 00:00:03,000\nb\n")
	f.Add("vtt", "WEBVTT\nLanguage: en\n\nNOTE\n00:01.000 --> 00:02.000\n<v A\tB>a <00:01.500>b\n")
	f.Add("ttml", `<tt xml:lang="en"><head><metadata><ttm:agent xml:id="v1"/></metadata></head>`+
		`<body><div><p begin="1s" ttm:agent="v1"><span begin="0s">a</span> <span begin="2s" end="1s">b</span></p></div></body></tt>`)
	f.Fuzz(func(t *testing.T, format, in string) {
		// Lenient input can keep invalid LRC timecodes as text, which the encoder doesn't escape, so the
		// output is read back with the same options.
		opt := options{voices: true, lenient: true}
		track, err := decode(strings.NewReader(in), format, opt)
		if err != nil {
			return
		}
		for _, out := range []string{"lrc", "srt", "vtt", "ttml"} {
			var b bytes.Buffer
			if err := encode(&b, track, out); err != nil {
				t.Fatalf("encoding %s: %s", out, err)
			}
			if _, err := decode(bytes.NewReader(b.Bytes()), out, opt); err != nil {
				t.Fatalf("decoding %s output: %s\n%q", out, err, b.String())
			}
		}
	})
}
