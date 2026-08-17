// Package parse turns what a person types into the values the model stores.
// Pure: no tea, no I/O.
package parse

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DateLayout is the only date format the app stores or renders: dd/mm/yy.
const DateLayout = "02/01/06"

var (
	ErrHours = errors.New("parse: not a duration")
	ErrDate  = errors.New("parse: not a date")
)

var hmRe = regexp.MustCompile(`^(?:([0-9]+(?:\.[0-9]+)?)h)?(?:([0-9]+(?:\.[0-9]+)?)m)?$`)

// Minutes reads a duration written any of the ways a person writes one:
//
//	7h30m -> 450   7.5 -> 450   90m -> 90   7:30 -> 450   7 -> 420   :30 -> 30
func Minutes(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, ErrHours
	}

	if strings.ContainsAny(s, "hm") {
		g := hmRe.FindStringSubmatch(s)
		if g == nil || (g[1] == "" && g[2] == "") {
			return 0, ErrHours
		}
		total := 0.0
		if g[1] != "" {
			h, err := strconv.ParseFloat(g[1], 64)
			if err != nil {
				return 0, ErrHours
			}
			total += h * 60
		}
		if g[2] != "" {
			m, err := strconv.ParseFloat(g[2], 64)
			if err != nil {
				return 0, ErrHours
			}
			total += m
		}
		return int(math.Round(total)), nil
	}

	if hs, ms, ok := strings.Cut(s, ":"); ok {
		hs, ms = strings.TrimSpace(hs), strings.TrimSpace(ms)
		// Either side may be left off — ":30" is half an hour, "7:" is seven of them.
		// A lone ":" says nothing and is refused.
		if hs == "" && ms == "" {
			return 0, ErrHours
		}
		h, m := 0, 0
		if hs != "" {
			if !digits(hs) {
				return 0, ErrHours
			}
			v, err := strconv.Atoi(hs)
			if err != nil {
				return 0, ErrHours
			}
			h = v
		}
		if ms != "" {
			// Minutes are two digits at most: ":005" is a typo, not five minutes, and
			// reading it as one would log hours nobody meant to log.
			if !digits(ms) || len(ms) > 2 {
				return 0, ErrHours
			}
			v, err := strconv.Atoi(ms)
			if err != nil || v > 59 {
				return 0, ErrHours
			}
			m = v
		}
		return h*60 + m, nil
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, ErrHours
	}
	return int(math.Round(f * 60)), nil
}

// digits reports whether s is nothing but ASCII digits. Atoi would accept "+30" and
// "-0" either side of the colon, which is not a time anybody types.
func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// FormatHM renders minutes for a table cell: 450 -> "7:30".
func FormatHM(min int) string {
	if min < 0 {
		min = 0
	}
	return fmt.Sprintf("%d:%02d", min/60, min%60)
}

// FormatTotal renders minutes for a total: 405 -> "6h45m", 420 -> "7h", 45 -> "45m".
func FormatTotal(min int) string {
	if min <= 0 {
		return "0m"
	}
	h, m := min/60, min%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh%dm", h, m)
	}
}

// Date reads a partial date against the date it replaces (base, dd/mm/yy):
//
//	8      -> 08/08/26   (day only: keep month and year)
//	8/9    -> 08/09/26   (day and month: keep year)
//	8/9/26 -> 08/09/26
//
// Empty input keeps base. An impossible date (31/02) is an error.
func Date(in, base string) (string, error) {
	b, err := time.Parse(DateLayout, strings.TrimSpace(base))
	if err != nil {
		b = time.Now()
	}
	in = strings.TrimSpace(in)
	if in == "" {
		return b.Format(DateLayout), nil
	}

	parts := strings.Split(in, "/")
	if len(parts) > 3 {
		return "", ErrDate
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return "", ErrDate
		}
		nums[i] = n
	}

	day, mon, yr := nums[0], int(b.Month()), b.Year()
	if len(nums) > 1 {
		mon = nums[1]
	}
	if len(nums) > 2 {
		yr = nums[2]
		if yr < 100 {
			yr += 2000
		}
	}

	t := time.Date(yr, time.Month(mon), day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day || int(t.Month()) != mon || t.Year() != yr {
		return "", ErrDate
	}
	return t.Format(DateLayout), nil
}

// DateMatches reports whether a stored date satisfies a partial query, comparing only
// the parts that were typed: "12" is the 12th of any month in any year, "12/7" any
// 12th of July, "12/7/26" that date alone. Leading zeros do not matter, so 7 and 07
// are the same month.
//
// This is what a jump inside a task's rows wants. Resolving "12" against today would
// pin it to this month, and the rows in front of you are not all from this month.
func DateMatches(date, q string) bool {
	want := strings.Split(strings.TrimSpace(q), "/")
	if want[0] == "" {
		return false
	}
	got := strings.Split(strings.TrimSpace(date), "/")
	for i, w := range want {
		if w == "" {
			continue // "12//26" — an unspecified middle part matches anything
		}
		if i >= len(got) || !sameNumber(w, got[i]) {
			return false
		}
	}
	return true
}

func sameNumber(a, b string) bool {
	x, errA := strconv.Atoi(strings.TrimSpace(a))
	y, errB := strconv.Atoi(strings.TrimSpace(b))
	return errA == nil && errB == nil && x == y
}

// Today is the date a new entry starts with.
func Today() string { return time.Now().Format(DateLayout) }

// Day reports the day-of-month of a stored date, or 0 if it is unreadable.
func Day(date string) int {
	t, err := time.Parse(DateLayout, strings.TrimSpace(date))
	if err != nil {
		return 0
	}
	return t.Day()
}
