package store

import (
	"os"
	"testing"
)

// The cache is a round trip: what is written comes back, and a missing file is a first run
// rather than an error.
func TestEmployeeCacheRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if msg := LoadEmployees().(EmployeesLoadedMsg); msg.Err != nil || len(msg.Employees) != 0 {
		t.Fatalf("a missing cache is an error: %+v", msg)
	}

	want := []Employee{
		{ID: 121, Name: "Abdul Alim Shohan", Job: "Software Engineer - L3",
			Email: "abdul.shohan@strativ.se"},
		{ID: 162, Name: "Abdullah Zayed", Job: "Software Engineer - L4",
			Email: "abdullah.zayed@strativ.se", Phone: "+46 72 130 50 43"},
	}
	if msg := SaveEmployees(want)().(SavedMsg); msg.Err != nil {
		t.Fatalf("Save: %v", msg.Err)
	}
	if _, err := os.Stat(EmployeesPath()); err != nil {
		t.Fatalf("nothing at %s: %v", EmployeesPath(), err)
	}
	got := LoadEmployees().(EmployeesLoadedMsg)
	if got.Err != nil || len(got.Employees) != 2 || got.Employees[1] != want[1] {
		t.Errorf("read back %+v", got)
	}
	// Its own file: writing the directory must not be able to lose the task list.
	if EmployeesPath() == Path() {
		t.Error("the directory shares the task file")
	}
}
