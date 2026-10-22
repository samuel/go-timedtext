package lrc

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

func TestDecodeCharset(t *testing.T) {
	enc := func(e encoding.Encoding, s string) []byte {
		b, err := e.NewEncoder().Bytes([]byte(s))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cases := []struct {
		name string
		in   []byte
		opt  DecodeOptions
		exp  string
	}{
		{"encoding tag", enc(simplifiedchinese.GBK, "[encoding: GBK]\n[00:01.00]你好"), DecodeOptions{}, "你好"},
		{"charset option", enc(japanese.ShiftJIS, "[00:01.00]こんにちは"), DecodeOptions{Charset: japanese.ShiftJIS}, "こんにちは"},
		{"UTF-16", enc(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), "[00:01.00]naïve"), DecodeOptions{}, "naïve"},
		{"Windows-1252", []byte("[00:01.00]caf\xe9"), DecodeOptions{}, "café"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lyr, err := Decode(bytes.NewReader(c.in), c.opt)
			if err != nil {
				t.Fatal(err)
			}
			exp := []Phrase{{Timecode: time.Second, Lines: []string{c.exp}}}
			if !reflect.DeepEqual(lyr.Phrases, exp) {
				t.Errorf("Phrases = %s, expected %s", formatPhrases(lyr.Phrases), formatPhrases(exp))
			}
		})
	}
}

func TestEncodeEncodingTag(t *testing.T) {
	lyr := &Lyrics{Tags: []Tag{{TagEncoding, "gbk"}}, Phrases: []Phrase{{Lines: []string{"你好"}}}}
	var b bytes.Buffer
	if err := Encode(&b, lyr, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertSameLines(t, b.String(), "[encoding: utf-8]\n\n[00:00.00]你好\n")
}

func TestEncodeCharset(t *testing.T) {
	gbk := simplifiedchinese.GBK
	in, err := gbk.NewEncoder().Bytes([]byte("[encoding: gbk]\n\n[00:01.00]你好\n"))
	if err != nil {
		t.Fatal(err)
	}
	det := DetectCharset(in, DecodeOptions{})
	if det.Encoding != gbk || det.BOM {
		t.Fatalf("DetectCharset() = %+v, expected GBK without BOM", det)
	}
	lyr, err := Decode(bytes.NewReader(in), DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := Encode(&b, lyr, EncodeOptions{Charset: det.Encoding, BOM: det.BOM}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), in) {
		t.Errorf("Encode() = %q, expected the original %q", b.Bytes(), in)
	}

	lyr.Phrases[0].Lines[0] = "こんにちは ☃"
	if err := Encode(&bytes.Buffer{}, lyr, EncodeOptions{Charset: gbk}); err == nil {
		t.Error("expected error for characters GBK can't represent")
	}
}

func TestEncodeBOM(t *testing.T) {
	lyr := &Lyrics{Phrases: []Phrase{{Lines: []string{"a"}}}}
	var b bytes.Buffer
	if err := Encode(&b, lyr, EncodeOptions{BOM: true}); err != nil {
		t.Fatal(err)
	}
	if exp := "\xEF\xBB\xBF[00:00.00]a\n"; b.String() != exp {
		t.Errorf("Encode() = %q, expected %q", b.String(), exp)
	}
}
