package lrc_test

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"samuelks.com/timedtext/lrc"
	"samuelks.com/timedtext/srt"
)

const exampleLRC = `[ti:Example Song]
[ar:Example Artist]

[00:01.00]first line
[00:04.50]<00:04.50>word <00:05.00>by <00:05.50>word
[00:08.00]
`

func ExampleDecode() {
	lyr, err := lrc.Decode(strings.NewReader(exampleLRC), lrc.DecodeOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(lyr.Title(), "-", lyr.Artist())
	for _, p := range lyr.Phrases {
		fmt.Printf("%v %q %d words\n", p.Timecode, strings.Join(p.Lines, " "), len(p.Words))
	}
	// Output:
	// Example Song - Example Artist
	// 1s "first line" 0 words
	// 4.5s "word by word" 3 words
	// 8s "" 0 words
}

func ExampleEncode() {
	lyr := &lrc.Lyrics{
		Tags: []lrc.Tag{{Key: lrc.TagTitle, Value: "Example Song"}},
		Phrases: []lrc.Phrase{
			{Timecode: time.Second, Lines: []string{"first line"}},
			{Timecode: 62*time.Second + 500*time.Millisecond, Lines: []string{"second line"}},
		},
	}
	if err := lrc.Encode(os.Stdout, lyr, lrc.EncodeOptions{}); err != nil {
		log.Fatal(err)
	}
	// Output:
	// [ti: Example Song]
	//
	// [00:01.00]first line
	// [01:02.50]second line
}

// Convert LRC to SRT through the format-neutral model.
func ExampleLyrics_ToTrack() {
	lyr, err := lrc.Decode(strings.NewReader(exampleLRC), lrc.DecodeOptions{})
	if err != nil {
		log.Fatal(err)
	}
	track, err := lyr.ToTrack()
	if err != nil {
		log.Fatal(err)
	}
	if err := srt.Encode(os.Stdout, srt.FromTrack(track)); err != nil {
		log.Fatal(err)
	}
	// Output:
	// 1
	// 00:00:01,000 --> 00:00:04,500
	// first line
	//
	// 2
	// 00:00:04,500 --> 00:00:08,000
	// word by word
}
