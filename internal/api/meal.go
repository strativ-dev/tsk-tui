package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// MealType is one serp.meal.type — breakfast, lunch, snacks — in the ERP's own sequence.
//
// Serving is the hour it is served at and Before how long ahead of that booking closes,
// both decimal hours in Dhaka time, so the cutoff is Serving-Before. They are read here
// because a calendar of meals has to say which of today's slots can still be acted on.
type MealType struct {
	ID      int
	Name    string
	Serving float64
	Before  float64
}

// MealBooking is one booked meal.
//
// Date is Odoo's own Date field, so it needs no zone conversion — unlike a datetime, whose
// day in UTC is not the day it was eaten on in Dhaka. Locked is is_locked_for_user, the
// ERP's own answer to "can this still be changed", which beats recomputing the cutoff here.
type MealBooking struct {
	ID     int
	Date   string // yyyy-mm-dd
	TypeID int
	Type   string
	Menu   string
	Locked bool
}

// MealMenu is what the canteen is serving on one day, for one meal type.
//
// Common is what everyone gets and Options is the pick, slash-separated, the same split the
// booking rows carry as common_items / available_options. Most days have only the one line;
// a lunch with a choice has both.
type MealMenu struct {
	Date    string // yyyy-mm-dd
	TypeID  int
	Type    string
	Common  string
	Options string
}

// MealMsg is one month of meals: the types the office serves, what is on offer, what is
// booked, and the days it serves nothing on. The four reads travel in one message because
// they are one calendar — the types landing without the bookings would draw a month of empty slots that
// only looked like a month nobody ate in.
type MealMsg struct {
	Year     int
	Month    time.Month
	Types    []MealType
	Bookings []MealBooking
	// Menus is what is on offer, whether or not it was booked: the answer to "should I book
	// Thursday", which the bars cannot give.
	Menus []MealMenu
	// Closed is get_unusual_days: weekends and public holidays together, keyed
	// yyyy-mm-dd. It is per office, and it is the one call that knows the canteen is shut.
	Closed map[string]bool
	Err    error
}

// FetchMeals is a tea.Cmd: everything the meal calendar draws, for one month.
//
// Four execute_kw calls behind one login, the way the time off screen reads its year. The
// web client reaches these over /web/dataset/call_kw with a session cookie; we have an API
// key, so they go the same execute_kw way as the rest of the app.
func FetchMeals(key, login, db string, year int, month time.Month) tea.Cmd {
	return func() tea.Msg {
		key, login, db = strings.TrimSpace(key), strings.TrimSpace(login), strings.TrimSpace(db)
		fail := func(err error) tea.Msg { return MealMsg{Year: year, Month: month, Err: err} }

		uid, err := connect(db, login, key)
		if err != nil {
			return fail(err)
		}
		types, err := mealTypes(db, uid, key)
		if err != nil {
			return fail(err)
		}
		// The viewed month, and the one after it: a booking range can run past the end of a
		// month, and the calendar draws both when it does. One wider range costs the same
		// three calls, where fetching the second month on demand would cost three more and a
		// screen that disagrees with itself until they land.
		first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		last := first.AddDate(0, 2, -1)
		booked, err := mealBookings(db, uid, key, uid, first, last)
		if err != nil {
			return fail(err)
		}
		closed, err := mealClosedDays(db, uid, key, first, last)
		if err != nil {
			return fail(err)
		}
		menus, err := mealMenus(db, uid, key, first, last)
		if err != nil {
			return fail(err)
		}
		return MealMsg{Year: year, Month: month, Types: types,
			Bookings: booked, Closed: closed, Menus: menus}
	}
}

// mealTypes reads the active meal types in the ERP's own order: the bars on a day are
// whatever the office serves, so nothing about breakfast, lunch or snacks is hardcoded.
func mealTypes(db string, uid int, key string) ([]MealType, error) {
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"serp.meal.type", "search_read",
		[]any{[]any{[]any{"active", "=", true}}},
		map[string]any{
			"fields": []string{"name", "serving_time", "allow_booking_before_hours"},
			"order":  "sequence asc, id asc",
		},
	})
	if err != nil {
		return nil, err
	}

	var rows []struct {
		ID      int      `json:"id"`
		Name    odooText `json:"name"`
		Serving float64  `json:"serving_time"`
		Before  float64  `json:"allow_booking_before_hours"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("bad meal-type result: %w", err)
	}
	out := make([]MealType, 0, len(rows))
	for _, r := range rows {
		out = append(out, MealType{ID: r.ID, Name: oneLine(string(r.Name)),
			Serving: r.Serving, Before: r.Before})
	}
	return out, nil
}

// mealBookings reads the month's bookings, mine only.
//
// The user_id clause is not optional: a key with the meal-admin group sees the whole
// office, and a canteen list with everyone's meals on it says nothing about what you are
// eating. Cancelled rows are dropped — a meal nobody is having would draw as one that is.
func mealBookings(db string, uid int, key string, user int, first, last time.Time) ([]MealBooking, error) {
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"serp.meal.booking", "search_read",
		[]any{[]any{
			[]any{"user_id", "=", user},
			[]any{"date", ">=", first.Format("2006-01-02")},
			[]any{"date", "<=", last.Format("2006-01-02")},
			[]any{"state", "=", "booked"},
		}},
		map[string]any{
			"fields": []string{"date", "meal_type_id", "state", "is_locked_for_user",
				"menu_item_name"},
			"order": "date asc, id asc",
		},
	})
	if err != nil {
		return nil, err
	}

	var rows []struct {
		ID     int      `json:"id"`
		Date   odooText `json:"date"`
		Type   odooRef  `json:"meal_type_id"`
		Locked bool     `json:"is_locked_for_user"`
		Menu   odooText `json:"menu_item_name"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("bad meal booking result: %w", err)
	}
	out := make([]MealBooking, 0, len(rows))
	for _, r := range rows {
		out = append(out, MealBooking{ID: r.ID, Date: strings.TrimSpace(string(r.Date)),
			TypeID: r.Type.ID, Type: oneLine(r.Type.Name), Locked: r.Locked,
			Menu: oneLine(string(r.Menu))})
	}
	return out, nil
}

// mealClosedDays asks the ERP which days it serves nothing on.
//
// get_unusual_days is an @api.model method taking the two ends and nothing else — no ids
// list, the same shape as the leave balances — and it answers with one bool per day:
// weekends and public holidays together, which is exactly the question "is there a canteen
// today". Working that out here would mean reading the office calendar and its holidays to
// say what one call already says, and getting it wrong on a holiday nobody told us about.
func mealClosedDays(db string, uid int, key string, first, last time.Time) (map[string]bool, error) {
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"serp.meal.booking", "get_unusual_days",
		[]any{
			first.Format("2006-01-02") + " 00:00:00",
			last.Format("2006-01-02") + " 23:59:59",
		},
		map[string]any{"context": map[string]any{"tz": "Asia/Dhaka", "lang": "en_US"}},
	})
	if err != nil {
		return nil, err
	}
	var days map[string]bool
	if err := json.Unmarshal(raw, &days); err != nil {
		return nil, fmt.Errorf("bad unusual-days result: %w", err)
	}
	return days, nil
}

// mealMenus reads what the canteen is serving, for the whole viewed month rather than one
// week: the panel follows the cursor and the cursor walks the month, so this is one call
// instead of one per week. There is no user in the domain — a menu is the same for everyone.
//
// A day with no rows is a day nothing was on offer, which is what the weekend looks like.
func mealMenus(db string, uid int, key string, first, last time.Time) ([]MealMenu, error) {
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"serp.meal.menu", "search_read",
		[]any{[]any{
			[]any{"date", ">=", first.Format("2006-01-02")},
			[]any{"date", "<=", last.Format("2006-01-02")},
		}},
		map[string]any{
			"fields": []string{"date", "meal_type_id", "common_items_display",
				"options_display"},
			"order": "date asc, meal_type_id asc",
		},
	})
	if err != nil {
		return nil, err
	}

	var rows []struct {
		Date    odooText `json:"date"`
		Type    odooRef  `json:"meal_type_id"`
		Common  odooText `json:"common_items_display"`
		Options odooText `json:"options_display"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("bad meal menu result: %w", err)
	}
	out := make([]MealMenu, 0, len(rows))
	for _, r := range rows {
		// oneLine like every other string the ERP wrote: these carry tabs and the odd stray
		// quote, and a newline in one would render the panel a row taller than its column.
		out = append(out, MealMenu{Date: strings.TrimSpace(string(r.Date)),
			TypeID: r.Type.ID, Type: oneLine(r.Type.Name),
			Common: oneLine(string(r.Common)), Options: oneLine(string(r.Options))})
	}
	return out, nil
}

// MealsDeletedMsg answers a cancellation with how many rows went.
type MealsDeletedMsg struct {
	Date string
	N    int
	Err  error
}

// CancelMeals is a tea.Cmd: unlink the given serp.meal.booking rows.
//
// An employee cancels a meal by deleting the row — only a meal admin may do it by setting
// state, so `write` is not the way in. Odoo answers unlink with a bare true/false, so false
// is a refusal rather than a success: the ERP's own rules still apply, and one past its
// cutoff stays booked.
//
// Nothing retries. A timed-out unlink that in fact landed would make the next attempt fail
// on a row that no longer exists, and the answer to that is to re-read, which the caller
// does anyway.
func CancelMeals(key, login, db, date string, ids []int) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg { return MealsDeletedMsg{Date: date, Err: err} }
		if len(ids) == 0 {
			return fail(errors.New("nothing booked on that day"))
		}
		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return fail(err)
		}
		raw, err := rpc("object", "execute_kw", []any{
			strings.TrimSpace(db), uid, strings.TrimSpace(key),
			"serp.meal.booking", "unlink",
			[]any{ids},
		})
		if err != nil {
			return fail(err)
		}
		var ok bool
		if err := json.Unmarshal(raw, &ok); err != nil || !ok {
			return fail(errors.New("the ERP would not cancel it"))
		}
		return MealsDeletedMsg{Date: date, N: len(ids)}
	}
}

// MealBookedMsg answers a booking with what the ERP took and what it refused.
//
// Booked and Skipped are counts because the request is a batch: one create per meal per day,
// and a day the canteen will not take is not a failure of the others. Why is the first
// refusal the ERP gave, verbatim — "already exists", "Booking is closed", and so on.
type MealBookedMsg struct {
	Booked  int
	Skipped int
	Why     string
	Err     error
}

// BookMeals is a tea.Cmd: one serp.meal.booking per meal per day.
//
// The rows are created one at a time on purpose. Odoo's create takes a list, but one refused
// row would roll the whole list back — and the refusals here are ordinary: a day already
// booked, a meal past its cutoff, a type not served that day. Booking four days and being
// told the fifth was full beats booking nothing.
//
// user_id is left out: it defaults to the caller, and naming it would let a key with the
// meal-admin group book for somebody else by accident. menu_id and menu_item_id resolve
// themselves from the date and the type, so they are not sent either.
func BookMeals(key, login, db string, days []string, types []int) tea.Cmd {
	return func() tea.Msg {
		key, login, db = strings.TrimSpace(key), strings.TrimSpace(login), strings.TrimSpace(db)
		fail := func(err error) tea.Msg { return MealBookedMsg{Err: err} }
		if len(days) == 0 || len(types) == 0 {
			return fail(errors.New("pick a day and a meal first"))
		}

		uid, err := connect(db, login, key)
		if err != nil {
			return fail(err)
		}

		out := MealBookedMsg{}
		for _, day := range days {
			for _, t := range types {
				raw, err := rpc("object", "execute_kw", []any{
					db, uid, key,
					"serp.meal.booking", "create",
					[]any{map[string]any{"meal_type_id": t, "date": day}},
					map[string]any{"context": map[string]any{
						"tz": "Asia/Dhaka", "lang": "en_US"}},
				})
				var id int
				switch {
				case err != nil:
					out.Skipped++
					if out.Why == "" {
						out.Why = oneLine(err.Error())
					}
				case json.Unmarshal(raw, &id) != nil || id == 0:
					out.Skipped++
					if out.Why == "" {
						out.Why = "the ERP would not take " + day
					}
				default:
					out.Booked++
				}
			}
		}
		return out
	}
}
