// Package charset detects the character encoding of text and converts between it and UTF-8.
package charset

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/unicode"
)

var (
	utf16LE = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	utf16BE = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
)

// Lookup returns the encoding for a name or label such as "utf-8", "gbk", "shift_jis", or "windows-1252".
func Lookup(name string) (encoding.Encoding, error) {
	enc, err := htmlindex.Get(name)
	if err != nil {
		return nil, fmt.Errorf("unknown character encoding %q", name)
	}
	return enc, nil
}

// Name returns the canonical name of an encoding, such as "gbk", or "" if it has none.
func Name(enc encoding.Encoding) string {
	name, err := htmlindex.Name(enc)
	if err != nil {
		return ""
	}
	return name
}

// Detected is the encoding of some text.
type Detected struct {
	Encoding encoding.Encoding // unicode.UTF8 for UTF-8
	BOM      bool              // The text starts with a byte order mark
}

// Detect returns the encoding of data. It is the first that applies of:
//
//  1. A UTF-8 or UTF-16 byte order mark.
//  2. UTF-16 without a byte order mark, recognized by a NUL in the first two bytes.
//  3. UTF-8, if data is valid UTF-8.
//  4. The encoding named by declared, the label a file gives for itself, if known. UTF-8 and UTF-16
//     labels are ignored, since the data has already been found not to be either.
//  5. fallback, if not nil.
//  6. Windows-1252, the most common encoding of older subtitle and lyrics files.
func Detect(data []byte, declared string, fallback encoding.Encoding) Detected {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return Detected{Encoding: unicode.UTF8, BOM: true}
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return Detected{Encoding: utf16LE, BOM: true}
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return Detected{Encoding: utf16BE, BOM: true}
	case len(data) >= 2 && data[0] != 0 && data[1] == 0:
		return Detected{Encoding: utf16LE}
	case len(data) >= 2 && data[0] == 0 && data[1] != 0:
		return Detected{Encoding: utf16BE}
	case utf8.Valid(data):
		return Detected{Encoding: unicode.UTF8}
	}
	if declared != "" {
		if enc, err := Lookup(declared); err == nil && enc != unicode.UTF8 && !isUTF16(enc) {
			return Detected{Encoding: enc}
		}
	}
	if fallback != nil {
		return Detected{Encoding: fallback}
	}
	return Detected{Encoding: charmap.Windows1252}
}

func isUTF16(enc encoding.Encoding) bool {
	name := Name(enc)
	return name == "utf-16le" || name == "utf-16be"
}

// Decode converts data to UTF-8 with any byte order mark removed, using the encoding from Detect.
func Decode(data []byte, declared string, fallback encoding.Encoding) ([]byte, error) {
	d := Detect(data, declared, fallback)
	if d.BOM {
		if d.Encoding == unicode.UTF8 {
			data = data[3:]
		} else {
			data = data[2:]
		}
	}
	if d.Encoding == unicode.UTF8 && utf8.Valid(data) {
		return data, nil
	}
	out, err := d.Encoding.NewDecoder().Bytes(data)
	if err != nil {
		return nil, fmt.Errorf("decoding text: %w", err)
	}
	return out, nil
}

// Encode converts UTF-8 text to enc, or leaves it as UTF-8 when enc is nil. With bom set, the output
// starts with a byte order mark, which only UTF encodings can represent. Characters that enc can't
// represent are an error.
func Encode(text []byte, enc encoding.Encoding, bom bool) ([]byte, error) {
	if bom {
		text = append([]byte{0xEF, 0xBB, 0xBF}, text...)
	}
	if enc == nil || enc == unicode.UTF8 {
		return text, nil
	}
	out, err := enc.NewEncoder().Bytes(text)
	if err != nil {
		if name := Name(enc); name != "" {
			return nil, fmt.Errorf("encoding text as %s: %w", name, err)
		}
		return nil, fmt.Errorf("encoding text: %w", err)
	}
	return out, nil
}
