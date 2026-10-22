# timedtext

[![Go Reference](https://pkg.go.dev/badge/samuelks.com/timedtext.svg)](https://pkg.go.dev/samuelks.com/timedtext)

Go packages for reading, writing, and converting timed text such as lyrics and subtitles.

```sh
go get samuelks.com/timedtext
```

API documentation and runnable examples are at
[pkg.go.dev/samuelks.com/timedtext](https://pkg.go.dev/samuelks.com/timedtext).

| Package | Format |
| --- | --- |
| `samuelks.com/timedtext` | Format-neutral model: `Track`, `Cue`, `Span`, `Metadata` |
| `samuelks.com/timedtext/lrc` | LRC lyrics, with the A2 (enhanced) and Walaoke extensions |
| `samuelks.com/timedtext/srt` | SubRip subtitles |
| `samuelks.com/timedtext/vtt` | WebVTT subtitles |
| `samuelks.com/timedtext/ttml` | The TTML subset used for lyrics by Apple Music and Spotify |
| `samuelks.com/timedtext/charset` | Character encoding detection and conversion |

Each format package has its own types that round-trip that format, and `ToTrack` / `FromTrack` to convert
to and from the shared model. The model holds timing, text, voice or speaker, timing within a cue, and basic
metadata. Styling and positioning stay in the format packages, so conversions drop them, along with anything
else the target format can't represent.

## Character encodings

Decoders convert input to UTF-8, so strings are always UTF-8. The encoding is the first that applies of:

1. A UTF-8 or UTF-16 byte order mark.
2. UTF-16 without a byte order mark.
3. UTF-8, if the input is valid UTF-8.
4. The encoding the file declares: the `[encoding:...]` tag in LRC, or the XML declaration in TTML. A
   declared UTF-8 or UTF-16 is ignored at this point, since the input has already been found not to be
   either.
5. `DecodeOptions.Charset`, for files such as GBK or Shift_JIS that don't declare one.
6. Windows-1252.

Encoders write UTF-8 by default, and LRC's encoding tag is written as `utf-8` to match. To keep a file's
original encoding, pass the result of `lrc.DetectCharset` (or `charset.Detect`) to
`lrc.EncodeOptions{Charset, BOM}`. The encoding tag is then written as is, and characters the encoding
can't represent are an error.

The `samuelks.com/timedtext/charset` package exposes the detection (`Detect`, `Decode`), conversion back
(`Encode`), and name lookup (`Lookup`, `Name`).

## Usage

```go
f, err := os.Open("song.lrc")
if err != nil {
	return err
}
defer f.Close()
lyr, err := lrc.Decode(f, lrc.DecodeOptions{})
if err != nil {
	return err
}
fmt.Println(lyr.Title(), lyr.Artist())
for _, p := range lyr.Phrases {
	fmt.Println(p.Timecode, strings.Join(p.Lines, " "))
}

// Convert to SRT.
track, err := lyr.ToTrack()
if err != nil {
	return err
}
return srt.Encode(os.Stdout, srt.FromTrack(track))
```

## Commands

```sh
go install samuelks.com/timedtext/cmd/...@latest
```

- `lrcfix [-n] [-utf8] [-charset name] file.lrc...` rewrites LRC files in place in the default format,
  keeping each file's character encoding and byte order mark. `-utf8` converts files to UTF-8 instead. It
  also fixes timecodes from tools that wrap minutes at 60 instead of carrying into hours: a drop of more
  than 30 minutes from the previous phrase adds an hour to that phrase and all later ones. Phrases from
  adjacent timecodes on one line (same text) count as one, files that already have a timecode of an hour
  or more are left alone, and word times that wrap within a phrase are fixed too. `-n` reports what would
  change without writing. A file whose text can't be written back in its encoding is left alone with an
  error. This usually means the encoding was misdetected, so set `-charset`, or use `-utf8`. Files are
  replaced atomically; a symbolic link's target is rewritten, and hard links are broken.
- `timedtext convert [-from format] [-to format] [-charset name] [-voices] [-lenient] in out` converts
  between `lrc`, `srt`, `vtt`, and `ttml`, chosen by extension (`.lrc`, `.srt`, `.vtt`, `.ttml`, `.xml`)
  unless `-from` or `-to` is set. `-` reads stdin or writes stdout, and needs the matching format flag.
  `-voices` reads Walaoke voice markers and `-lenient` keeps invalid timecodes as text in LRC input. The
  output file is replaced only when the conversion succeeds.

`-charset` takes a name such as `gbk`, `shift_jis`, or `iso-8859-2` for input that doesn't declare its
encoding.

## LRC syntax

Decoding accepts:

- ID tags such as `[ar:Artist]` before the first lyric line. Keys are lowercased and values trimmed. Later
  tag-shaped lines are lyric text. Tags keep their order, and repeated keys are kept.
- Timecodes `[mm:ss]`, `[mm:ss.f]`, `[hh:mm:ss.f]`, `[mm:ss:cc]`, and `[hh:mm:ss:cc]`. The fraction after `.`
  is scaled by its digit count, so `.5` is 500ms and `.123` is 123ms. With three colon-separated fields and
  no `.`, the last field is read as centiseconds when it has two digits, and the fields as `hh:mm:ss`
  otherwise.
- Repeated timecodes: `[00:21.10][00:45.10]chorus` gives both phrases the text. A2 word times are shifted
  for each copy by its distance from the first timecode.
- Several phrases on one line: `[00:18.07]you[00:18.15] write`.
- Untimed lines after a phrase, which add lines to it.
- A2 word timing: `[00:01.00]<00:01.00>Hel<00:01.20>lo <00:01.50>world`.
- Walaoke voice markers `M:`, `F:`, and `D:` at the start of a phrase, with `DecodeOptions.Voices`. A
  voice carries over to later phrases until the next marker.

Lines may end in `\n`, `\r\n`, or `\r`. `DecodeOptions.Lenient` keeps invalid timecodes as text instead
of failing. Errors are `*lrc.SyntaxError` with a line number.

The `offset` tag isn't applied on decode. Use `Lyrics.ApplyOffset` to shift the timecodes and remove the
tag. `ToTrack` applies it, sorts phrases by time, and ignores an invalid `length` tag.

Encoding writes tags as `[key: value]` in order, a blank line, then one `[mm:ss.cc]text` line per phrase.
Timecodes are rounded to centiseconds and minutes may exceed 59. Two non-standard options change this:

- `HoursInTimecode` writes `[hh:mm:ss.cc]` for timecodes of an hour or more.
- `Multiline` writes the timecode on its own line followed by each text line, with a blank line between
  phrases.

Line breaks in tag values are written as spaces. Text that starts with something shaped like a timecode
or tag isn't escaped, so it decodes differently.

## Other formats

- SRT: the index line is optional, and settings may follow the end time after a space or tab. Text after
  a blank line that isn't followed by a timing line is added to the cue before it, since some files have
  blank lines inside cues. An index that isn't the cue's position becomes the cue ID in the model.
- WebVTT: NOTE, STYLE, and REGION blocks round-trip. The `Language:` header line maps to the model's
  language. Cue IDs that would read back as one of those blocks are dropped when converting from the
  model.
- TTML: the lyrics subset, plus Apple's `iTunesMetadata` songwriters, which map to the model's author.

All decoders accept `\n`, `\r\n`, and `\r` line endings.

## References

- https://en.wikipedia.org/wiki/LRC_(file_format)
- https://docs.fileformat.com/misc/lrc/
- http://p.cweiske.de/458
- https://www.w3.org/TR/webvtt1/
- https://www.w3.org/TR/ttml2/

## License

MIT. See [LICENSE](LICENSE).
