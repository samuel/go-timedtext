package charset

import (
	"bytes"
	"testing"

	"golang.org/x/text/encoding/charmap"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

func encode(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	b, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecode(t *testing.T) {
	const text = "[ti:Café]\n[00:01.00]naïve"
	gbk := encode(t, simplifiedchinese.GBK, "[00:01.00]你好")
	cases := []struct {
		name     string
		in       []byte
		declared string
		fallback encoding.Encoding
		exp      string
	}{
		{"UTF-8", []byte(text), "", nil, text},
		{"UTF-8 BOM", append([]byte{0xEF, 0xBB, 0xBF}, text...), "", nil, text},
		{"UTF-16LE BOM", encode(t, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), text), "", nil, text},
		{"UTF-16BE BOM", encode(t, unicode.UTF16(unicode.BigEndian, unicode.UseBOM), text), "", nil, text},
		{"UTF-16LE", encode(t, unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), text), "", nil, text},
		{"UTF-16BE", encode(t, unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), text), "", nil, text},
		{"Windows-1252 default", []byte("caf\xe9"), "", nil, "café"},
		{"declared", gbk, "gbk", nil, "[00:01.00]你好"},
		{"unknown declared uses fallback", gbk, "nope", simplifiedchinese.GBK, "[00:01.00]你好"},
		{"fallback", encode(t, japanese.ShiftJIS, "こんにちは"), "", japanese.ShiftJIS, "こんにちは"},
		{"declared UTF-8 but invalid", []byte("caf\xe9"), "utf-8", nil, "café"},
		{"declared UTF-16 without NULs", []byte("caf\xe9"), "utf-16", nil, "café"},
		{"declared UTF-16BE without NULs", []byte("caf\xe9"), "utf-16be", nil, "café"},
		{"valid UTF-8 ignores fallback", []byte(text), "gbk", japanese.ShiftJIS, text},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := Decode(c.in, c.declared, c.fallback)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != c.exp {
				t.Errorf("Decode() = %q, expected %q", out, c.exp)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	for _, name := range []string{"utf-8", "GBK", "shift_jis", "windows-1252", "latin1"} {
		if _, err := Lookup(name); err != nil {
			t.Errorf("Lookup(%q) failed: %s", name, err)
		}
	}
	if _, err := Lookup("nope"); err == nil {
		t.Error("expected error for unknown encoding")
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name     string
		in       []byte
		declared string
		exp      Detected
	}{
		{"UTF-8", []byte("abc"), "", Detected{Encoding: unicode.UTF8}},
		{"UTF-8 BOM", []byte("\xEF\xBB\xBFabc"), "", Detected{Encoding: unicode.UTF8, BOM: true}},
		{"UTF-16LE BOM", []byte("\xFF\xFEa\x00"), "", Detected{Encoding: utf16LE, BOM: true}},
		{"UTF-16BE", []byte("\x00a"), "", Detected{Encoding: utf16BE}},
		{"declared", []byte("\xc4\xe3"), "gbk", Detected{Encoding: simplifiedchinese.GBK}},
		{"default", []byte("caf\xe9"), "", Detected{Encoding: charmap.Windows1252}},
	}
	for _, c := range cases {
		if d := Detect(c.in, c.declared, nil); d != c.exp {
			t.Errorf("%s: Detect() = %+v, expected %+v", c.name, d, c.exp)
		}
	}
}

func TestEncode(t *testing.T) {
	cases := []struct {
		name string
		enc  encoding.Encoding
		bom  bool
		exp  []byte
	}{
		{"UTF-8", nil, false, []byte("naïve")},
		{"UTF-8 BOM", unicode.UTF8, true, []byte("\xEF\xBB\xBFnaïve")},
		{"UTF-16LE BOM", utf16LE, true, encode(t, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), "naïve")},
		{"Windows-1252", charmap.Windows1252, false, []byte("na\xefve")},
	}
	for _, c := range cases {
		out, err := Encode([]byte("naïve"), c.enc, c.bom)
		if err != nil {
			t.Errorf("%s: %s", c.name, err)
		} else if !bytes.Equal(out, c.exp) {
			t.Errorf("%s: Encode() = %q, expected %q", c.name, out, c.exp)
		}
	}
	if _, err := Encode([]byte("你好"), charmap.Windows1252, false); err == nil {
		t.Error("expected error for unrepresentable characters")
	}
	if _, err := Encode([]byte("a"), charmap.Windows1252, true); err == nil {
		t.Error("expected error for a BOM in Windows-1252")
	}
}

func TestRoundTrip(t *testing.T) {
	for _, enc := range []encoding.Encoding{simplifiedchinese.GBK, japanese.ShiftJIS, utf16BE} {
		in := encode(t, enc, "[00:01.00]こんにちは")
		d := Detect(in, "", enc)
		text, err := Decode(in, "", enc)
		if err != nil {
			t.Fatal(err)
		}
		out, err := Encode(text, d.Encoding, d.BOM)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, in) {
			t.Errorf("%s: round trip = %q, expected %q", Name(enc), out, in)
		}
	}
}

func TestName(t *testing.T) {
	if n := Name(simplifiedchinese.GBK); n != "gbk" {
		t.Errorf("Name(GBK) = %q", n)
	}
	if n := Name(utf16LE); n != "utf-16le" {
		t.Errorf("Name(UTF-16LE) = %q", n)
	}
}
