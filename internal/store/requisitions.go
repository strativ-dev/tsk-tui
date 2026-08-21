package store

// Prop is one of a requisition's own properties: what the category asked for. Odoo answers a
// properties field with the definition and the value together, so the label is the ERP's own
// ("Purpose of Replacement") and the kind is what says how to draw the value.
type Prop struct {
	Label string `json:"label"`
	Kind  string `json:"kind"` // char, text, boolean, date, many2one, selection, …
	Value string `json:"value"`
}

// Requisition is one row of the requisitions tab: what was asked for, for whom, and where the
// approval has got to.
//
// The properties differ per category — a replacement asks for the purpose and the spec, a
// maintenance one asks something else — so they are carried as a list rather than as fields
// this struct would have to know the names of.
type Requisition struct {
	ID          int    `json:"id"`
	Category    string `json:"category"`
	Submitted   string `json:"submitted"` // dd/mm/yy
	Deadline    string `json:"deadline"`
	For         string `json:"for"`
	Designation string `json:"designation"`
	Stage       string `json:"stage"`
	Urgent      bool   `json:"urgent"`
	Urgency     string `json:"urgency"`
	Note        string `json:"note"`
	Props       []Prop `json:"props"`
}
