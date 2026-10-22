// Package lines splits text into lines.
package lines

import (
	"iter"
	"strings"
)

// All yields each line of s with its 1-based number. Lines end with "\n", "\r\n", or "\r", and the
// endings are removed. A line ending at the end of s doesn't start another line.
func All(s string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		for n := 1; s != ""; n++ {
			i := strings.IndexAny(s, "\r\n")
			if i < 0 {
				yield(n, s)
				return
			}
			line := s[:i]
			if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			s = s[i+1:]
			if !yield(n, line) {
				return
			}
		}
	}
}
