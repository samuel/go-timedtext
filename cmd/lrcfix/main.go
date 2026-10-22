// Command lrcfix normalizes LRC files in place. It fixes timecodes from writers that wrap minutes at 60
// instead of carrying into hours, and re-encodes each file in the default format. Files keep their
// character encoding and byte order mark unless -utf8 is set.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"golang.org/x/text/encoding/unicode"
	"samuelks.com/timedtext/charset"
	"samuelks.com/timedtext/internal/atomicfile"
	"samuelks.com/timedtext/lrc"
)

// wrapThreshold is how far a timecode must drop from the previous one to count as an hour wrap.
// Smaller drops are treated as intentionally out-of-order phrases.
const wrapThreshold = 30 * time.Minute

func main() {
	var opt options
	flag.BoolVar(&opt.dryRun, "n", false, "report what would change without writing")
	flag.BoolVar(&opt.toUTF8, "utf8", false, "convert files to UTF-8 instead of keeping their encoding")
	charsetName := flag.String("charset", "", "encoding of input that isn't UTF-8 or UTF-16 and has no encoding tag (default windows-1252)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: lrcfix [-n] [-utf8] [-charset name] file.lrc...\n") //nolint:errcheck // nothing to do if usage can't be printed
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *charsetName != "" {
		enc, err := charset.Lookup(*charsetName)
		if err != nil {
			slog.Error("Invalid charset", "error", err)
			os.Exit(2)
		}
		opt.decode.Charset = enc
	}
	var failed bool
	for _, path := range flag.Args() {
		res, err := rewrite(path, opt)
		if err != nil {
			slog.Error("Failed to rewrite", "path", path, "error", err)
			failed = true
			continue
		}
		if res.changed {
			msg := "Rewrote"
			if opt.dryRun {
				msg = "Would rewrite"
			}
			slog.Info(msg, "path", path, "charset", res.charset, "hourWrapsFixed", res.hourWrapsFixed)
		}
	}
	if failed {
		os.Exit(1)
	}
}

type options struct {
	decode lrc.DecodeOptions
	toUTF8 bool // Convert to UTF-8 instead of keeping the file's encoding
	dryRun bool
}

type result struct {
	changed        bool   // The output differs from the input
	charset        string // Encoding of the output
	hourWrapsFixed bool
}

// rewrite fixes and re-encodes the file at path. It leaves the file alone if nothing changes or
// opt.dryRun is set.
func rewrite(path string, opt options) (result, error) {
	in, err := os.ReadFile(path) //nolint:gosec // The user names the files to rewrite
	if err != nil {
		return result{}, err
	}
	lyr, err := lrc.Decode(bytes.NewReader(in), opt.decode)
	if err != nil {
		return result{}, err
	}
	res := result{hourWrapsFixed: fixHourWraps(lyr.Phrases), charset: "utf-8"}
	var eopt lrc.EncodeOptions
	if !opt.toUTF8 {
		det := lrc.DetectCharset(in, opt.decode)
		eopt.BOM = det.BOM
		if det.Encoding != unicode.UTF8 {
			eopt.Charset = det.Encoding
			res.charset = charset.Name(det.Encoding)
		}
	}
	var out bytes.Buffer
	if err := lrc.Encode(&out, lyr, eopt); err != nil {
		if eopt.Charset != nil {
			return result{}, fmt.Errorf("%w (the file may not be %s; set -charset, or -utf8 to convert it)", err, res.charset)
		}
		return result{}, err
	}
	res.changed = !bytes.Equal(in, out.Bytes())
	if !res.changed || opt.dryRun {
		return res, nil
	}
	return res, atomicfile.Write(path, out.Bytes(), 0o644)
}

// fixHourWraps corrects timecodes from writers that wrap minutes at 60 instead of carrying into hours.
// It reports whether any timecode changed. Files with a timecode of an hour or more weren't written by
// such a writer and are left alone.
func fixHourWraps(phrases []lrc.Phrase) bool {
	for _, p := range phrases {
		if p.Timecode >= time.Hour || slices.ContainsFunc(p.Words, func(w lrc.Word) bool { return w.Timecode >= time.Hour }) {
			return false
		}
	}
	var offset, last time.Duration
	var fixed bool
	var prevLines []string
	for i := range phrases {
		p := &phrases[i]
		raw := p.Timecode
		// Adjacent timecodes on one line become consecutive phrases with the same text, and can be far
		// apart, so only the first of them is compared.
		if len(p.Lines) == 0 || !slices.Equal(p.Lines, prevLines) {
			if last-raw > wrapThreshold {
				offset += time.Hour
			}
			last = raw
		}
		prevLines = p.Lines
		for j := range p.Words {
			w := &p.Words[j]
			wordOffset := offset
			if raw-w.Timecode > wrapThreshold {
				wordOffset += time.Hour
			}
			if wordOffset != 0 {
				w.Timecode += wordOffset
				fixed = true
			}
		}
		if offset != 0 {
			p.Timecode += offset
			fixed = true
		}
	}
	return fixed
}
