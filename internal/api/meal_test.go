package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeMeals answers the four reads the meal calendar makes, and records every call so the
// arguments can be checked — the shapes are what a real ERP refuses when they are wrong.
func fakeMeals(t *testing.T, calls *[]map[string]any) {
	t.Helper()
	const types = `[
		{"id": 6, "name": "Snacks", "serving_time": 17.5, "allow_booking_before_hours": 8.0},
		{"id": 2, "name": "Breakfast", "serving_time": 10.5, "allow_booking_before_hours": 1.0}
	]`
	const bookings = `[
		{"id": 35862, "date": "2026-08-14", "meal_type_id": [2, "Breakfast"],
			"state": "booked", "is_locked_for_user": true,
			"menu_item_name": "ruti,\naloo bhaji"},
		{"id": 35863, "date": "2026-08-14", "meal_type_id": [6, "Snacks"],
			"state": "booked", "is_locked_for_user": false, "menu_item_name": false}
	]`
	const unusual = `{"2026-08-01": true, "2026-08-03": false, "2026-08-05": true}`
	const menus = `[
		{"id": 167, "date": "2026-08-17", "meal_type_id": [2, "Breakfast"],
			"common_items_display": false, "options_display": "paratha,\tomlet"},
		{"id": 166, "date": "2026-08-17", "meal_type_id": [1, "Lunch"],
			"common_items_display": "rice, dal, salad",
			"options_display": "hash bhuna / chicken bhuna"}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Service string `json:"service"`
				Method  string `json:"method"`
				Args    []any  `json:"args"`
			} `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("unparsable request: %v", err)
		}
		if calls != nil {
			*calls = append(*calls, map[string]any{
				"service": req.Params.Service,
				"method":  req.Params.Method,
				"args":    req.Params.Args,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		result := `[]`
		if req.Params.Method == "login" {
			result = `26`
		}
		if len(req.Params.Args) >= 5 {
			model, _ := req.Params.Args[3].(string)
			method, _ := req.Params.Args[4].(string)
			switch {
			case model == "serp.meal.type":
				result = types
			case model == "serp.meal.booking" && method == "search_read":
				result = bookings
			case model == "serp.meal.booking" && method == "get_unusual_days":
				result = unusual
			case model == "serp.meal.menu":
				result = menus
			}
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)
}

func TestFetchMeals(t *testing.T) {
	var calls []map[string]any
	fakeMeals(t, &calls)

	msg, ok := FetchMeals("secret-key", "user@example.com", "erp-test", 2026, time.August)().(MealMsg)
	if !ok {
		t.Fatal("FetchMeals did not return MealMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if msg.Year != 2026 || msg.Month != time.August {
		t.Errorf("month = %d %v", msg.Year, msg.Month)
	}

	// The types come back in the order the ERP sent them — the bars on a day are laid out in
	// that order, so re-sorting here would move them.
	if len(msg.Types) != 2 || msg.Types[0].Name != "Snacks" || msg.Types[0].ID != 6 {
		t.Errorf("types = %+v", msg.Types)
	}
	if got := msg.Types[1]; got.Serving != 10.5 || got.Before != 1 {
		t.Errorf("breakfast cutoff figures = %+v", got)
	}

	// Bookings: a menu name with a newline in it is one line by the time it reaches the
	// view, and an empty one arrives as Odoo's false rather than as a string.
	if len(msg.Bookings) != 2 {
		t.Fatalf("bookings = %d, want 2", len(msg.Bookings))
	}
	if got := msg.Bookings[0]; got.TypeID != 2 || got.Type != "Breakfast" ||
		got.Date != "2026-08-14" || !got.Locked || got.Menu != "ruti, aloo bhaji" {
		t.Errorf("bookings[0] = %+v", got)
	}
	if got := msg.Bookings[1]; got.Menu != "" || got.Locked {
		t.Errorf("bookings[1] = %+v, want an empty menu and unlocked", got)
	}

	// The menus: what is on offer whether or not it was booked, with the choice and the
	// common items kept apart, and an empty one arriving as Odoo's false rather than a string.
	if len(msg.Menus) != 2 {
		t.Fatalf("menus = %d, want 2", len(msg.Menus))
	}
	if got := msg.Menus[0]; got.Date != "2026-08-17" || got.TypeID != 2 ||
		got.Common != "" || got.Options != "paratha, omlet" {
		t.Errorf("menus[0] = %+v", got)
	}
	if got := msg.Menus[1]; got.Common != "rice, dal, salad" ||
		got.Options != "hash bhuna / chicken bhuna" {
		t.Errorf("menus[1] = %+v", got)
	}

	// The closed days are the ERP's own answer: weekends and holidays together, which is why
	// 5 August — a Wednesday — is in there.
	if !msg.Closed["2026-08-05"] || msg.Closed["2026-08-03"] {
		t.Errorf("closed = %v", msg.Closed)
	}
}

// A booking is one create per meal per day, with the caller left implicit and the menu left
// to resolve itself, and a refused row does not take the others with it.
func TestBookMeals(t *testing.T) {
	var calls []map[string]any
	fakeMeals(t, &calls)

	msg, ok := BookMeals("secret-key", "user@example.com", "erp-test",
		[]string{"2026-08-24", "2026-08-25"}, []int{2, 1})().(MealBookedMsg)
	if !ok {
		t.Fatal("BookMeals did not return MealBookedMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	// The fake answers create with an empty list, which is neither an id nor an error: two
	// days by two meals is four attempts either way, which is what the counts have to add to.
	if msg.Booked+msg.Skipped != 4 {
		t.Errorf("booked %d, skipped %d, want four attempts", msg.Booked, msg.Skipped)
	}

	var creates []map[string]any
	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 6 {
			continue
		}
		if model, _ := args[3].(string); model != "serp.meal.booking" {
			continue
		}
		if method, _ := args[4].(string); method != "create" {
			continue
		}
		// create takes its values inside a list: args = [vals], the way Odoo's own client
		// sends it.
		inner, _ := args[5].([]any)
		if len(inner) == 0 {
			t.Errorf("create was called with no values: %v", args[5])
			continue
		}
		vals, _ := inner[0].(map[string]any)
		creates = append(creates, vals)
	}
	if len(creates) != 4 {
		t.Fatalf("%d create calls, want one per meal per day", len(creates))
	}
	for _, v := range creates {
		if _, named := v["user_id"]; named {
			// It defaults to the caller; naming it lets a meal-admin key book for somebody
			// else by accident.
			t.Errorf("create names a user: %v", v)
		}
		for _, computed := range []string{"menu_id", "menu_item_id", "state"} {
			if _, sent := v[computed]; sent {
				t.Errorf("create sends %s, which resolves itself: %v", computed, v)
			}
		}
		if v["meal_type_id"] == nil || v["date"] == nil {
			t.Errorf("create is missing the meal or the day: %v", v)
		}
	}

	// Nothing to do is refused before the round trip.
	if got := BookMeals("k", "l", "db", nil, []int{2})().(MealBookedMsg); got.Err == nil {
		t.Error("a booking with no day was sent")
	}
	if got := BookMeals("k", "l", "db", []string{"2026-08-24"}, nil)().(MealBookedMsg); got.Err == nil {
		t.Error("a booking with no meal was sent")
	}
}

// The two argument shapes a real ERP is strict about: the bookings must be scoped to the
// caller, and get_unusual_days takes the two ends with no ids list in front of them.
func TestMealCallShapes(t *testing.T) {
	var calls []map[string]any
	fakeMeals(t, &calls)
	FetchMeals("secret-key", "user@example.com", "erp-test", 2026, time.August)()

	var domain, unusual, menus []any
	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 6 {
			continue
		}
		model, _ := args[3].(string)
		method, _ := args[4].(string)
		switch {
		case model == "serp.meal.booking" && method == "search_read":
			inner, _ := args[5].([]any)
			if len(inner) > 0 {
				domain, _ = inner[0].([]any)
			}
		case model == "serp.meal.booking" && method == "get_unusual_days":
			unusual, _ = args[5].([]any)
		case model == "serp.meal.menu":
			inner, _ := args[5].([]any)
			if len(inner) > 0 {
				menus, _ = inner[0].([]any)
			}
		}
	}

	// A meal-admin key sees the whole office, so the user clause is what keeps the calendar
	// about you. Nothing else in the app can supply it after the fact.
	var scoped bool
	for _, clause := range domain {
		parts, _ := clause.([]any)
		if len(parts) == 3 && parts[0] == "user_id" && parts[1] == "=" {
			scoped = true
		}
	}
	if !scoped {
		t.Errorf("the booking domain is not scoped to the user: %v", domain)
	}

	// Two datetimes and nothing else: an ids list in front of them fails the call, the same
	// way it does for the leave balances.
	if len(unusual) != 2 {
		t.Fatalf("get_unusual_days args = %v, want the two ends alone", unusual)
	}
	// The viewed month and the one after it: a booking range can cross the end of a month, and
	// the calendar draws both when it does.
	if unusual[0] != "2026-08-01 00:00:00" || unusual[1] != "2026-09-30 23:59:59" {
		t.Errorf("get_unusual_days spans %v, want the month and the next", unusual)
	}

	// The menus span the whole month, not one week: the panel follows the cursor and the
	// cursor walks the month, so this is one call instead of one per week. And no user in the
	// domain — a menu is the same for everyone.
	if len(menus) != 2 {
		t.Fatalf("menu domain = %v, want the two ends of the month", menus)
	}
	for _, clause := range menus {
		parts, _ := clause.([]any)
		if len(parts) == 3 && parts[0] == "user_id" {
			t.Errorf("the menu domain is scoped to a user: %v", menus)
		}
	}
}
