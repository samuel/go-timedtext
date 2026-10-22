package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"

	"samuelks.com/timedtext/lrc"
)

func TestFixHourWraps(t *testing.T) {
	m := func(minutes, seconds int) time.Duration {
		return time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
	}
	cases := []struct {
		name  string
		in    []time.Duration
		text  []string // Text of each phrase, if any
		exp   []time.Duration
		fixed bool
	}{
		{
			name: "no wrap",
			in:   []time.Duration{m(0, 1), m(30, 0), m(59, 59)},
			exp:  []time.Duration{m(0, 1), m(30, 0), m(59, 59)},
		},
		{
			name:  "one wrap",
			in:    []time.Duration{m(59, 58), m(0, 1), m(0, 5)},
			exp:   []time.Duration{m(59, 58), m(60, 1), m(60, 5)},
			fixed: true,
		},
		{
			name:  "two wraps",
			in:    []time.Duration{m(59, 58), m(0, 1), m(59, 59), m(0, 2)},
			exp:   []time.Duration{m(59, 58), m(60, 1), m(119, 59), m(120, 2)},
			fixed: true,
		},
		{
			name: "out of order",
			in:   []time.Duration{m(0, 21), m(0, 45), m(0, 30)},
			exp:  []time.Duration{m(0, 21), m(0, 45), m(0, 30)},
		},
		{
			name: "repeated timecodes far apart",
			in:   []time.Duration{m(0, 20), m(59, 30), m(0, 25)},
			text: []string{"chorus", "chorus", "verse"},
			exp:  []time.Duration{m(0, 20), m(59, 30), m(0, 25)},
		},
		{
			name:  "wrap after repeated timecodes",
			in:    []time.Duration{m(59, 50), m(59, 55), m(0, 5)},
			text:  []string{"chorus", "chorus", "verse"},
			exp:   []time.Duration{m(59, 50), m(59, 55), m(60, 5)},
			fixed: true,
		},
		{
			name: "already past an hour",
			in:   []time.Duration{m(59, 58), m(60, 1), m(0, 5)},
			exp:  []time.Duration{m(59, 58), m(60, 1), m(0, 5)},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			phrases := make([]lrc.Phrase, len(c.in))
			for i, tc := range c.in {
				phrases[i] = lrc.Phrase{Timecode: tc}
				if c.text != nil {
					phrases[i].Lines = []string{c.text[i]}
				}
			}
			if fixed := fixHourWraps(phrases); fixed != c.fixed {
				t.Errorf("fixHourWraps returned %t, expected %t", fixed, c.fixed)
			}
			for i, p := range phrases {
				if p.Timecode != c.exp[i] {
					t.Errorf("phrase %d = %s, expected %s", i, p.Timecode, c.exp[i])
				}
			}
		})
	}
}

func TestFixHourWrapsWords(t *testing.T) {
	m := func(minutes, seconds int) time.Duration {
		return time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
	}
	phrases := []lrc.Phrase{
		{Timecode: m(59, 59), Words: []lrc.Word{{Timecode: m(59, 59)}, {Timecode: m(0, 1)}}},
		{Timecode: m(0, 2), Words: []lrc.Word{{Timecode: m(0, 2)}}},
	}
	if !fixHourWraps(phrases) {
		t.Error("fixHourWraps returned false")
	}
	var got []time.Duration
	for _, p := range phrases {
		for _, w := range p.Words {
			got = append(got, w.Timecode)
		}
	}
	if exp := []time.Duration{m(59, 59), m(60, 1), m(60, 2)}; !slices.Equal(got, exp) {
		t.Errorf("word timecodes = %v, expected %v", got, exp)
	}
	if tc := phrases[1].Timecode; tc != m(60, 2) {
		t.Errorf("phrase 1 = %s, expected 1h0m2s", tc)
	}
}

func TestRewriteSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "song.lrc")
	if err := os.WriteFile(target, []byte("[00:01]a"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.lrc")
	if err := os.Symlink("song.lrc", link); err != nil {
		t.Fatal(err)
	}
	if _, err := rewrite(link, options{}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "[00:01.00]a\n" {
		t.Errorf("target = %q, %v", b, err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symbolic link replaced: %v, %v", info, err)
	}
}

func TestRewrite(t *testing.T) {
	const in = "[ti:Song]\n[59:58.00]a\n[00:01.00]b\n"
	path := filepath.Join(t.TempDir(), "song.lrc")
	if err := os.WriteFile(path, []byte(in), 0o640); err != nil {
		t.Fatal(err)
	}
	res, err := rewrite(path, options{dryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if exp := (result{changed: true, charset: "utf-8", hourWrapsFixed: true}); res != exp {
		t.Errorf("dry run result = %+v, expected %+v", res, exp)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != in {
		t.Fatalf("dry run changed the file: %q, %v", b, err)
	}
	if _, err := rewrite(path, options{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if exp := "[ti: Song]\n\n[59:58.00]a\n[60:01.00]b\n"; string(b) != exp {
		t.Errorf("rewritten file = %q, expected %q", b, exp)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Errorf("mode = %o, expected 640", perm)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only the rewritten file, found %d entries", len(entries))
	}
	res, err = rewrite(path, options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.changed {
		t.Error("rewriting a normalized file reported a change")
	}
}

func TestRewriteCharset(t *testing.T) {
	gbk := simplifiedchinese.GBK
	enc := func(e encoding.Encoding, s string) []byte {
		b, err := e.NewEncoder().Bytes([]byte(s))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
	cases := []struct {
		name    string
		in      []byte
		opt     options
		exp     []byte
		charset string
	}{
		{
			name:    "GBK kept",
			in:      enc(gbk, "[encoding:gbk]\n[59:58.00]你\n[00:01.00]好\n"),
			exp:     enc(gbk, "[encoding: gbk]\n\n[59:58.00]你\n[60:01.00]好\n"),
			charset: "gbk",
		},
		{
			name:    "charset option kept",
			in:      enc(gbk, "[00:01]你好"),
			opt:     options{decode: lrc.DecodeOptions{Charset: gbk}},
			exp:     enc(gbk, "[00:01.00]你好\n"),
			charset: "gbk",
		},
		{
			name:    "UTF-16 with BOM kept",
			in:      enc(utf16, "[00:01]naïve"),
			exp:     enc(utf16, "[00:01.00]naïve\n"),
			charset: "utf-16le",
		},
		{
			name:    "UTF-8 BOM kept",
			in:      []byte("\xEF\xBB\xBF[00:01]a"),
			exp:     []byte("\xEF\xBB\xBF[00:01.00]a\n"),
			charset: "utf-8",
		},
		{
			name:    "converted to UTF-8",
			in:      enc(gbk, "[encoding:gbk]\n[00:01.00]你好\n"),
			opt:     options{toUTF8: true},
			exp:     []byte("[encoding: utf-8]\n\n[00:01.00]你好\n"),
			charset: "utf-8",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "song.lrc")
			if err := os.WriteFile(path, c.in, 0o644); err != nil {
				t.Fatal(err)
			}
			res, err := rewrite(path, c.opt)
			if err != nil {
				t.Fatal(err)
			}
			if res.charset != c.charset {
				t.Errorf("charset = %q, expected %q", res.charset, c.charset)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(b, c.exp) {
				t.Errorf("rewritten file = %q, expected %q", b, c.exp)
			}
		})
	}
}

func TestRewriteUnrepresentable(t *testing.T) {
	// 0x81 is undefined in Windows-1252, so it decodes to U+FFFD, which can't be written back.
	in := []byte("[00:01]caf\xe9 \x81")
	path := filepath.Join(t.TempDir(), "song.lrc")
	if err := os.WriteFile(path, in, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rewrite(path, options{}); err == nil || !strings.Contains(err.Error(), "-utf8") {
		t.Fatalf("expected an error suggesting -utf8, got %v", err)
	}
	if b, err := os.ReadFile(path); err != nil || !bytes.Equal(b, in) {
		t.Fatalf("file changed after a failed rewrite: %q, %v", b, err)
	}
	if _, err := rewrite(path, options{toUTF8: true}); err != nil {
		t.Fatal(err)
	}
}
