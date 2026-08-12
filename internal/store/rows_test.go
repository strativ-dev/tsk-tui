package store

import "testing"

func TestMergeRows(t *testing.T) {
	remote := []Entry{
		{ID: 90211, Date: "12/08/26", Desc: "pulled two", Minutes: 150},
		{ID: 90210, Date: "10/08/26", Desc: "pulled one", Minutes: 75},
	}
	local := []Entry{
		{ID: -1, Date: "11/08/26", Desc: "typed here", Minutes: 120, Local: true},
		{ID: 90211, Date: "12/08/26", Desc: "edited pulled two", Minutes: 200, Local: true},
		{ID: 90210, Date: "10/08/26", Desc: "stale copy of pulled one", Minutes: 1},
	}

	got := MergeRows(local, remote)
	if len(got) != 3 {
		t.Fatalf("merged %d rows, want 3:\n%+v", len(got), got)
	}
	// Newest first.
	if got[0].Date != "12/08/26" || got[1].Date != "11/08/26" || got[2].Date != "10/08/26" {
		t.Errorf("wrong order: %s, %s, %s", got[0].Date, got[1].Date, got[2].Date)
	}
	if got[0].Desc != "edited pulled two" {
		t.Errorf("a local edit must beat the pulled copy, got %q", got[0].Desc)
	}
	if got[1].Desc != "typed here" {
		t.Errorf("an app-created entry must survive the pull, got %q", got[1].Desc)
	}
	if got[2].Desc != "pulled one" || got[2].Minutes != 75 {
		t.Errorf("a non-local row must take Odoo's version, got %+v", got[2])
	}
}

func TestMergeRowsEmptyRemote(t *testing.T) {
	local := []Entry{{ID: -1, Date: "12/08/26", Desc: "typed here", Minutes: 60, Local: true}}
	if got := MergeRows(local, nil); len(got) != 1 || got[0].Desc != "typed here" {
		t.Errorf("a task with no Odoo lines must keep its local rows, got %+v", got)
	}
	if got := MergeRows([]Entry{{ID: 5, Date: "12/08/26"}}, nil); len(got) != 0 {
		t.Errorf("a pulled row absent from Odoo is gone, got %+v", got)
	}
}

func TestNextEntryIDStaysNegative(t *testing.T) {
	task := Task{Rows: []Entry{{ID: 90211}, {ID: -1}}}
	if got := NextEntryID(task); got != -2 {
		t.Errorf("NextEntryID = %d, want -2 so it cannot collide with an Odoo id", got)
	}
	if got := NextEntryID(Task{}); got != -1 {
		t.Errorf("NextEntryID on an empty task = %d, want -1", got)
	}
}
