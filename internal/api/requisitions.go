package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/store"
)

// RequisitionsMsg is the requisitions the key's owner has filed.
type RequisitionsMsg struct {
	Rows []store.Requisition
	Err  error
}

// reqLimit is what the ERP's own list view asks for, and enough for anyone's history here.
const reqLimit = 80

// FetchRequisitions is a tea.Cmd: the requisitions filed for the key's owner, newest first.
//
// One call. The web client reads the same model over /web/dataset/call_kw with
// web_search_read; search_read is the same query, and the domain is the one its list view
// sends — `employee_id.user_id = uid`, so this is your own requisitions and not the office's.
//
// **The detail comes with the list**, not on expanding a row: `requisition_properties` is one
// field on the same record, so asking for it here costs nothing and opening a row costs no
// round trip. That is the opposite of the employee directory, where the detail is a different
// read and the ids in it have to be resolved.
func FetchRequisitions(key, login, db string) tea.Cmd {
	return func() tea.Msg {
		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return RequisitionsMsg{Err: err}
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, "serp.general.requisition", "search_read",
			[]any{[]any{[]any{"employee_id.user_id", "=", uid}}},
			map[string]any{
				"fields": []string{"requisition_category_id", "submission_date", "deadline",
					"employee_id", "employee_designation", "waiting_stage_name", "is_urgent",
					"urgency_cause", "note", "requisition_properties"},
				"order": "submission_date desc, id desc",
				"limit": reqLimit,
				// The properties carry their own labels, which are translated server-side.
				"context": map[string]any{"lang": "en_US", "tz": "Asia/Dhaka"},
			},
		})
		if err != nil {
			return RequisitionsMsg{Err: err}
		}

		var rows []struct {
			ID          int        `json:"id"`
			Category    odooRef    `json:"requisition_category_id"`
			Submitted   odooText   `json:"submission_date"`
			Deadline    odooText   `json:"deadline"`
			For         odooRef    `json:"employee_id"`
			Designation odooRef    `json:"employee_designation"`
			Stage       odooText   `json:"waiting_stage_name"`
			Urgent      bool       `json:"is_urgent"`
			Urgency     odooText   `json:"urgency_cause"`
			Note        odooText   `json:"note"`
			Props       []odooProp `json:"requisition_properties"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return RequisitionsMsg{Err: fmt.Errorf("bad requisition result: %w", err)}
		}

		out := make([]store.Requisition, 0, len(rows))
		for _, r := range rows {
			req := store.Requisition{
				ID:          r.ID,
				Category:    oneLine(r.Category.Name),
				Submitted:   isoToDate(string(r.Submitted)),
				Deadline:    isoToDate(string(r.Deadline)),
				For:         oneLine(r.For.Name),
				Designation: oneLine(r.Designation.Name),
				Stage:       oneLine(string(r.Stage)),
				Urgent:      r.Urgent,
				Urgency:     oneLine(string(r.Urgency)),
				// oneLine like everything else the ERP wrote: a note here runs to several
				// paragraphs, and a newline in a value grows the row it is drawn on.
				Note: oneLine(string(r.Note)),
			}
			for _, p := range r.Props {
				if v := p.text(); v != "" {
					req.Props = append(req.Props, store.Prop{
						Label: oneLine(p.String), Kind: p.Type, Value: v,
					})
				}
			}
			out = append(out, req)
		}
		return RequisitionsMsg{Rows: out}
	}
}

// odooProp is one entry of a properties field: the category's own definition and the value
// together, which is how Odoo answers one.
type odooProp struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	String string          `json:"string"`
	Value  json.RawMessage `json:"value"`
}

// text is the property's value as a line of text, by its own type. An empty one comes back
// empty and is left off the card rather than drawn as a label with nothing after it.
func (p odooProp) text() string {
	switch p.Type {
	case "boolean":
		var b bool
		_ = json.Unmarshal(p.Value, &b)
		if b {
			return "yes"
		}
		return "no"
	case "date", "datetime":
		var s odooText
		_ = json.Unmarshal(p.Value, &s)
		return isoToDate(string(s))
	case "many2one":
		// A pair, and its name is null when the caller cannot read the record's own name —
		// which is most of maintenance.equipment for a normal user, so the id stands in.
		var ref odooRef
		_ = json.Unmarshal(p.Value, &ref)
		if ref.Name != "" {
			return oneLine(ref.Name)
		}
		if ref.ID != 0 {
			return "#" + strconv.Itoa(ref.ID)
		}
		return ""
	case "many2many", "tags":
		var refs []odooRef
		if err := json.Unmarshal(p.Value, &refs); err == nil {
			var names []string
			for _, r := range refs {
				if r.Name != "" {
					names = append(names, oneLine(r.Name))
				}
			}
			return strings.Join(names, ", ")
		}
		return ""
	case "integer", "float", "monetary":
		var f float64
		if err := json.Unmarshal(p.Value, &f); err != nil {
			return ""
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	var s odooText
	_ = json.Unmarshal(p.Value, &s)
	return oneLine(string(s))
}

// isoToDate turns Odoo's own YYYY-MM-DD into the dd/mm/yy this app writes dates in. Odoo's
// false for an empty date arrives as an empty string and stays one.
func isoToDate(s string) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(strings.SplitN(s, " ", 2)[0]))
	if err != nil {
		return ""
	}
	return t.Format("02/01/06")
}

// ReqCategoriesMsg is the kinds of requisition you can file, each with the fields it asks for.
type ReqCategoriesMsg struct {
	Categories []store.ReqCategory
	Err        error
}

// FetchReqCategories is a tea.Cmd: the categories, and what each one asks for.
//
// The fields come from `requisition_properties_definition` on the **category** — a properties
// field keeps its definition on the parent record and its values on the child, which is why the
// form can be built from this one read and why a category nobody has taught this app about still
// asks the right questions.
//
// A many2one field's options are **not** read here: which comodel it points at is in the
// definition, and reading every one of them for a category nobody opened would be several calls
// for a form nobody is filling. FetchReqOptions does it when a category is chosen.
func FetchReqCategories(key, login, db string) tea.Cmd {
	return func() tea.Msg {
		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return ReqCategoriesMsg{Err: err}
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, "serp.general.requisition.category", "search_read",
			[]any{[]any{[]any{"active", "=", true}}},
			map[string]any{
				"fields":  []string{"name", "requisition_properties_definition"},
				"order":   "name asc",
				"limit":   reqLimit,
				"context": map[string]any{"lang": "en_US", "tz": "Asia/Dhaka"},
			},
		})
		if err != nil {
			return ReqCategoriesMsg{Err: err}
		}

		var rows []struct {
			ID     int      `json:"id"`
			Name   odooText `json:"name"`
			Fields []struct {
				Name     string `json:"name"`
				Type     string `json:"type"`
				String   string `json:"string"`
				Required bool   `json:"is_required"`
				Comodel  string `json:"comodel"`
			} `json:"requisition_properties_definition"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return ReqCategoriesMsg{Err: fmt.Errorf("bad category result: %w", err)}
		}

		out := make([]store.ReqCategory, 0, len(rows))
		for _, r := range rows {
			cat := store.ReqCategory{ID: r.ID, Name: oneLine(string(r.Name))}
			for _, f := range r.Fields {
				cat.Fields = append(cat.Fields, store.ReqField{
					Name: f.Name, Kind: f.Type, Label: oneLine(f.String),
					Required: f.Required, Comodel: f.Comodel,
				})
			}
			out = append(out, cat)
		}
		return ReqCategoriesMsg{Categories: out}
	}
}

// ReqOptionsMsg is what one many2one field on the form can be set to.
type ReqOptionsMsg struct {
	Field   string // the property's own name, so a late answer lands on the right field
	Options []store.Opt
	Err     error
}

// FetchReqOptions is a tea.Cmd: the records a many2one field on the form points at.
//
// Whatever the caller may read, which is the point — maintenance.equipment answers with the
// five devices that are yours, not the office's inventory, because that is what a record rule
// on that model already decides.
func FetchReqOptions(key, login, db, field, model string) tea.Cmd {
	return func() tea.Msg {
		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return ReqOptionsMsg{Field: field, Err: err}
		}
		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, model, "search_read",
			[]any{[]any{}},
			map[string]any{
				"fields":  []string{"id", "name"},
				"order":   "name asc",
				"limit":   reqLimit,
				"context": map[string]any{"lang": "en_US"},
			},
		})
		if err != nil {
			return ReqOptionsMsg{Field: field, Err: err}
		}
		var rows []struct {
			ID   int      `json:"id"`
			Name odooText `json:"name"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return ReqOptionsMsg{Field: field, Err: fmt.Errorf("bad %s result: %w", model, err)}
		}
		opts := make([]store.Opt, 0, len(rows))
		for _, r := range rows {
			opts = append(opts, store.Opt{ID: r.ID, Name: oneLine(string(r.Name))})
		}
		return ReqOptionsMsg{Field: field, Options: opts}
	}
}

// RequisitionFiledMsg answers the create.
type RequisitionFiledMsg struct {
	ID  int
	Err error
}

// FileRequisition is a tea.Cmd: file one requisition.
//
// One create, and no submit after it: this model has no action_submit — `is_submitted` is
// computed, and a created record is already in the first approval stage, which is what the
// stage column then says. **Nothing retries**: a timed-out create that in fact landed would ask
// the office for the same thing twice, and a duplicate requisition is a conversation with HR.
//
// The properties go in as the whole list, definition and value together, which is how Odoo
// writes a properties field and what its own web client sends: name and type are what tell the
// server which definition a value belongs to.
func FileRequisition(key, login, db string, employee, category int,
	props []store.PropValue, urgent bool, urgency, note string) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg { return RequisitionFiledMsg{Err: err} }
		if category == 0 {
			return fail(errors.New("pick a category first"))
		}
		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return fail(err)
		}

		values := make([]map[string]any, 0, len(props))
		for _, p := range props {
			values = append(values, map[string]any{
				"name": p.Name, "type": p.Kind, "string": p.Label, "value": p.Value,
			})
		}
		vals := map[string]any{
			"requisition_category_id": category,
			"requisition_properties":  values,
			"is_urgent":               urgent,
		}
		if employee != 0 {
			// The same hr.employee the rest of the app read for this key; the field defaults to
			// it anyway, so this only saves the default a lookup.
			vals["employee_id"] = employee
		}
		if note = strings.TrimSpace(note); note != "" {
			vals["note"] = note
		}
		if urgency = strings.TrimSpace(urgency); urgent && urgency != "" {
			vals["urgency_cause"] = urgency
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, "serp.general.requisition", "create", []any{vals},
			map[string]any{"context": map[string]any{"lang": "en_US", "tz": "Asia/Dhaka"}},
		})
		if err != nil {
			return fail(err)
		}
		var id int
		if err := json.Unmarshal(raw, &id); err != nil || id == 0 {
			return fail(errors.New("the ERP refused the requisition"))
		}
		return RequisitionFiledMsg{ID: id}
	}
}
