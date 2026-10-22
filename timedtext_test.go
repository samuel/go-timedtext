package timedtext

import (
	"reflect"
	"testing"
	"time"
)

func TestCueEnd(t *testing.T) {
	tr := &Track{Cues: []Cue{
		{Start: 1 * time.Second, End: 2 * time.Second},
		{Start: 3 * time.Second},
		{Start: 3 * time.Second},
		{Start: 10 * time.Second},
		{Start: 20 * time.Second},
		{Start: 15 * time.Second},
		{Start: 30 * time.Second, End: 30 * time.Second},
	}}
	exp := []time.Duration{
		2 * time.Second,
		10 * time.Second,
		10 * time.Second,
		20 * time.Second,
		20*time.Second + DefaultCueDuration,
		30 * time.Second,
		30*time.Second + DefaultCueDuration,
	}
	for i, e := range exp {
		if end := tr.CueEnd(i); end != e {
			t.Errorf("CueEnd(%d) = %s, expected %s", i, end, e)
		}
	}
}

func TestSort(t *testing.T) {
	tr := &Track{Cues: []Cue{
		{Start: 2 * time.Second, Text: "b"},
		{Start: time.Second, Text: "a1"},
		{Start: time.Second, Text: "a2"},
	}}
	tr.Sort()
	var got []string
	for _, c := range tr.Cues {
		got = append(got, c.Text)
	}
	if exp := []string{"a1", "a2", "b"}; !reflect.DeepEqual(got, exp) {
		t.Errorf("sorted = %q, expected %q", got, exp)
	}
}
