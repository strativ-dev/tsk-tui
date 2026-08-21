package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The requisition read: the ERP's own list domain, and a properties field turned into labelled
// lines. The properties differ per category, so nothing here names one.
func TestFetchRequisitions(t *testing.T) {
	const rows = `[{
		"id": 367,
		"requisition_category_id": [5, "Accessories Replacement Requisition"],
		"submission_date": "2026-02-25",
		"deadline": "2026-02-24",
		"employee_id": [16, "Md. Tasnim Alam"],
		"employee_designation": [152, "Senior Software Engineer"],
		"waiting_stage_name": "Rejected",
		"is_urgent": true,
		"urgency_cause": "Broken on a client call",
		"note": "Model: FB35CS\nsilent switch",
		"requisition_properties": [
			{"name": "deadline", "type": "date", "string": "Deadline", "value": "2026-02-24"},
			{"name": "existing_device_id", "type": "many2one", "string": "Existing Device",
				"comodel": "maintenance.equipment", "value": [26, null]},
			{"name": "purpose_of_replacement", "type": "char",
				"string": "Purpose of Replacement", "value": "I need a headphone"},
			{"name": "specification", "type": "char", "string": "Specification", "value": ""},
			{"name": "is_data_backed_up", "type": "boolean", "string": "Is Data Backed Up",
				"value": true}
		]
	}, {
		"id": 198,
		"requisition_category_id": [3, "Device Maintenance Requisition"],
		"submission_date": "2025-11-30",
		"deadline": false,
		"employee_id": [16, "Md. Tasnim Alam"],
		"employee_designation": false,
		"waiting_stage_name": "Rejected",
		"is_urgent": false,
		"urgency_cause": false,
		"note": false,
		"requisition_properties": []
	}]`

	var domain []any
	var kwargs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Method string `json:"method"`
				Args   []any  `json:"args"`
			} `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		result := rows
		if req.Params.Method == "login" {
			result = `26`
		} else if len(req.Params.Args) >= 7 {
			if inner, ok := req.Params.Args[5].([]any); ok && len(inner) > 0 {
				domain, _ = inner[0].([]any)
			}
			kwargs, _ = req.Params.Args[6].(map[string]any)
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)

	msg, ok := FetchRequisitions("k", "user@example.com", "db")().(RequisitionsMsg)
	if !ok {
		t.Fatal("FetchRequisitions did not return RequisitionsMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if len(msg.Rows) != 2 {
		t.Fatalf("%d rows", len(msg.Rows))
	}

	r := msg.Rows[0]
	// Dates come back as Odoo writes them and are rendered the way this app writes dates.
	if r.Submitted != "25/02/26" || r.Deadline != "24/02/26" {
		t.Errorf("dates = %q → %q", r.Submitted, r.Deadline)
	}
	if r.Category != "Accessories Replacement Requisition" || r.Stage != "Rejected" ||
		r.Designation != "Senior Software Engineer" || !r.Urgent ||
		r.Urgency != "Broken on a client call" {
		t.Errorf("row = %+v", r)
	}
	// The note keeps its text on one line: a newline in it would grow the row it is drawn on.
	if r.Note != "Model: FB35CS silent switch" {
		t.Errorf("note = %q", r.Note)
	}

	// The properties: labels from the ERP, values by their own type, and an empty one left out.
	want := []struct{ label, value string }{
		{"Deadline", "24/02/26"},
		{"Existing Device", "#26"}, // a name the caller cannot read, so the id stands in
		{"Purpose of Replacement", "I need a headphone"},
		{"Is Data Backed Up", "yes"},
	}
	if len(r.Props) != len(want) {
		t.Fatalf("props = %+v, want %d of them", r.Props, len(want))
	}
	for i, w := range want {
		if r.Props[i].Label != w.label || r.Props[i].Value != w.value {
			t.Errorf("props[%d] = %+v, want %q = %q", i, r.Props[i], w.label, w.value)
		}
	}

	// Odoo's false, everywhere it can appear: a missing deadline, designation, cause and note.
	if second := msg.Rows[1]; second.Deadline != "" || second.Designation != "" ||
		second.Urgency != "" || second.Note != "" || len(second.Props) != 0 {
		t.Errorf("rows[1] = %+v", second)
	}

	// The ERP's own list domain: your requisitions, not the office's.
	clause, _ := domain[0].([]any)
	if len(domain) != 1 || len(clause) != 3 || clause[0] != "employee_id.user_id" ||
		clause[1] != "=" {
		t.Errorf("domain = %v, want the caller's own", domain)
	}
	if kwargs["order"] != "submission_date desc, id desc" {
		t.Errorf("order = %v", kwargs["order"])
	}
	// The properties come with the list, so opening a row costs no round trip.
	fields, _ := kwargs["fields"].([]any)
	var asked bool
	for _, f := range fields {
		if f == "requisition_properties" {
			asked = true
		}
	}
	if !asked {
		t.Errorf("the read does not ask for the properties: %v", fields)
	}
}
