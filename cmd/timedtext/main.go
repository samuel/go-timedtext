// Command timedtext converts between lyrics and subtitle formats.
//
//	timedtext convert [-from format] [-to format] [-charset name] [-voices] [-lenient] input output
//
// Formats are lrc, srt, vtt, and ttml, chosen by file extension (.lrc, .srt, .vtt, and .ttml or .xml)
// unless -from or -to is set. An input or output of "-" is stdin or stdout, which needs -from or -to.
// Input in other character encodings is converted to UTF-8, and output is always UTF-8. The output file
// is replaced only after the conversion succeeds.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding"
	"samuelks.com/timedtext"
	"samuelks.com/timedtext/charset"
	"samuelks.com/timedtext/internal/atomicfile"
	"samuelks.com/timedtext/lrc"
	"samuelks.com/timedtext/srt"
	"samuelks.com/timedtext/ttml"
	"samuelks.com/timedtext/vtt"
)

// Standard streams, replaced in tests.
var (
	stdin  io.Reader = os.Stdin
	stdout io.Writer = os.Stdout
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: timedtext convert [-from format] [-to format] [-charset name] [-voices] [-lenient] input output\n\n"+
		"Formats are lrc, srt, vtt, and ttml, chosen by extension (.lrc, .srt, .vtt, .ttml, .xml)\n"+
		"unless -from or -to is set. Use - for stdin or stdout.\n")
}

// options holds the settings for a conversion.
type options struct {
	from, to string // Formats, or empty to choose by extension
	charset  encoding.Encoding
	voices   bool
	lenient  bool
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "convert" {
		usage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet("convert", flag.ExitOnError)
	var opt options
	fs.StringVar(&opt.from, "from", "", "input format (default from the input's extension)")
	fs.StringVar(&opt.to, "to", "", "output format (default from the output's extension)")
	charsetName := fs.String("charset", "", "encoding of input that isn't UTF-8 or UTF-16 and doesn't declare one (default windows-1252)")
	fs.BoolVar(&opt.voices, "voices", false, "parse Walaoke M:, F:, and D: prefixes in LRC input")
	fs.BoolVar(&opt.lenient, "lenient", false, "keep invalid timecodes in LRC input as text")
	fs.Usage = func() {
		usage()
		fs.PrintDefaults()
	}
	fs.Parse(os.Args[2:]) //nolint:errcheck,gosec // ExitOnError exits instead of returning an error
	if fs.NArg() != 2 {
		fs.Usage()
		os.Exit(2)
	}
	if *charsetName != "" {
		enc, err := charset.Lookup(*charsetName)
		if err != nil {
			slog.Error("Invalid charset", "error", err)
			os.Exit(2)
		}
		opt.charset = enc
	}
	in, out := fs.Arg(0), fs.Arg(1)
	if err := convert(in, out, opt); err != nil {
		slog.Error("Conversion failed", "input", in, "output", out, "error", err)
		os.Exit(1)
	}
}

func convert(inPath, outPath string, opt options) error {
	inFmt, err := formatFor(inPath, opt.from, "-from")
	if err != nil {
		return err
	}
	outFmt, err := formatFor(outPath, opt.to, "-to")
	if err != nil {
		return err
	}
	track, err := decodeFile(inPath, inFmt, opt)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	if err := encode(&b, track, outFmt); err != nil {
		return err
	}
	if outPath == "-" {
		_, err := stdout.Write(b.Bytes())
		return err
	}
	return atomicfile.Write(outPath, b.Bytes(), 0o644)
}

// formatFor returns the format named by name, or by the extension of path when name is empty.
func formatFor(path, name, flagName string) (string, error) {
	if name != "" {
		f, ok := formatOf("." + name)
		if !ok {
			return "", fmt.Errorf("unknown format %q", name)
		}
		return f, nil
	}
	if path == "-" {
		return "", fmt.Errorf("%s is needed with -", flagName)
	}
	f, ok := formatOf(filepath.Ext(path))
	if !ok {
		return "", fmt.Errorf("unknown format for %s; set %s", path, flagName)
	}
	return f, nil
}

// formatOf returns the format for a file extension.
func formatOf(ext string) (string, bool) {
	switch ext = strings.ToLower(ext); ext {
	case ".lrc", ".srt", ".vtt", ".ttml":
		return ext[1:], true
	case ".xml":
		return "ttml", true
	}
	return "", false
}

func decodeFile(path, format string, opt options) (*timedtext.Track, error) {
	if path == "-" {
		return decode(stdin, format, opt)
	}
	f, err := os.Open(path) //nolint:gosec // The user names the input file
	if err != nil {
		return nil, err
	}
	track, err := decode(f, format, opt)
	if closeErr := f.Close(); err == nil && closeErr != nil {
		return nil, closeErr
	}
	return track, err
}

func decode(r io.Reader, format string, opt options) (*timedtext.Track, error) {
	switch format {
	case "lrc":
		l, err := lrc.Decode(r, lrc.DecodeOptions{Voices: opt.voices, Lenient: opt.lenient, Charset: opt.charset})
		if err != nil {
			return nil, err
		}
		return l.ToTrack()
	case "srt":
		s, err := srt.Decode(r, srt.DecodeOptions{Charset: opt.charset})
		if err != nil {
			return nil, err
		}
		return s.ToTrack(), nil
	case "vtt":
		v, err := vtt.Decode(r, vtt.DecodeOptions{Charset: opt.charset})
		if err != nil {
			return nil, err
		}
		return v.ToTrack(), nil
	case "ttml":
		d, err := ttml.Decode(r, ttml.DecodeOptions{Charset: opt.charset})
		if err != nil {
			return nil, err
		}
		return d.ToTrack(), nil
	}
	return nil, errors.New("unknown format " + format)
}

func encode(w io.Writer, track *timedtext.Track, format string) error {
	switch format {
	case "lrc":
		return lrc.Encode(w, lrc.FromTrack(track), lrc.EncodeOptions{})
	case "srt":
		return srt.Encode(w, srt.FromTrack(track))
	case "vtt":
		return vtt.Encode(w, vtt.FromTrack(track))
	case "ttml":
		return ttml.Encode(w, ttml.FromTrack(track))
	}
	return errors.New("unknown format " + format)
}
