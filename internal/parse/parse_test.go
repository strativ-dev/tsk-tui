package parse

import "testing"

func TestMinutes(t *testing.T) {
	ok := []struct {
		in   string
		want int
	}{
		{"7h30m", 450},
		{"7h", 420},
		{"30m", 30},
		{"90m", 90},
		{"7.5", 450},
		{"7:30", 450},
		{"0:45", 45},
		{"7", 420},
		{" 7H30M ", 450},
		{"0", 0},
		{"1.25", 75},
	}
	for _, c := range ok {
		got, err := Minutes(c.in)
		if err != nil {
			t.Errorf("Minutes(%q) = error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Minutes(%q) = %d, want %d", c.in, got, c.want)
		}
	}

	bad := []string{"", "abc", "7:75", "-3", "h", "7h30", "1:2:3", "7m30h"}
	for _, in := range bad {
		if got, err := Minutes(in); err == nil {
			t.Errorf("Minutes(%q) = %d, want error", in, got)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		min     int
		hm, tot string
	}{
		{450, "7:30", "7h30m"},
		{420, "7:00", "7h"},
		{405, "6:45", "6h45m"},
		{45, "0:45", "45m"},
		{0, "0:00", "0m"},
	}
	for _, c := range cases {
		if got := FormatHM(c.min); got != c.hm {
			t.Errorf("FormatHM(%d) = %q, want %q", c.min, got, c.hm)
		}
		if got := FormatTotal(c.min); got != c.tot {
			t.Errorf("FormatTotal(%d) = %q, want %q", c.min, got, c.tot)
		}
	}
}

func TestDate(t *testing.T) {
	const base = "12/08/26" // 12 Aug 2026

	ok := []struct {
		in, want string
	}{
		{"8", "08/08/26"},
		{"8/9", "08/09/26"},
		{"8/9/26", "08/09/26"},
		{"08/09/2026", "08/09/26"},
		{"", base},
		{" 1 / 1 ", "01/01/26"},
		{"31/12", "31/12/26"},
	}
	for _, c := range ok {
		got, err := Date(c.in, base)
		if err != nil {
			t.Errorf("Date(%q) = error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Date(%q, %q) = %q, want %q", c.in, base, got, c.want)
		}
	}

	bad := []string{"31/02", "0/1", "8/13", "abc", "1/2/3/4", "8/x"}
	for _, in := range bad {
		if got, err := Date(in, base); err == nil {
			t.Errorf("Date(%q) = %q, want error", in, got)
		}
	}
}

// DateMatches compares only what was typed: a jump inside a task looks at rows from
// several months, so "12" cannot mean "the 12th of this month".
func TestDateMatches(t *testing.T) {
	cases := []struct {
		date, q string
		want    bool
	}{
		{"12/08/26", "12", true},
		{"12/07/26", "12", true}, // any month, which is the point
		{"12/07/25", "12", true}, // any year too
		{"13/08/26", "12", false},
		{"12/07/26", "12/7", true},
		{"12/07/26", "12/07", true}, // a leading zero is the same month
		{"12/08/26", "12/7", false},
		{"12/07/26", "12/7/26", true},
		{"12/07/25", "12/7/26", false},
		{"12/07/26", "12//26", true}, // unspecified middle part matches anything
		{"12/08/26", "", false},      // nothing typed matches nothing
		{"12/08/26", "  ", false},
		{"12/08/26", "1", false}, // 1 is not 12
		{"", "12", false},
		{"12/08/26", "x", false},
	}
	for _, c := range cases {
		if got := DateMatches(c.date, c.q); got != c.want {
			t.Errorf("DateMatches(%q, %q) = %v, want %v", c.date, c.q, got, c.want)
		}
	}
}
