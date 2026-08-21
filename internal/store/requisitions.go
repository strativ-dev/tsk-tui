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

// ReqField is one field a requisition category asks for, as the category itself defines it:
// Odoo keeps the definition on the parent record and the values on the requisition, so this is
// half of a properties field.
//
// Comodel is set on a many2one — the model its options come from — and Opts are those options
// once they have been read.
type ReqField struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"` // char, text, date, boolean, integer, float, many2one, selection
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Comodel  string `json:"comodel"`
	Opts     []Opt  `json:"opts"`
}

// Opt is one choice of a many2one or selection field: what to send, and what to show.
type Opt struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ReqCategory is a kind of requisition you can file, and the fields it asks for.
type ReqCategory struct {
	ID     int        `json:"id"`
	Name   string     `json:"name"`
	Fields []ReqField `json:"fields"`
}

// PropValue is one field of a filed requisition: the definition Odoo wants echoed back, and
// the value. Odoo writes a properties field as the whole list, definition included, which is
// what its own web client sends.
type PropValue struct {
	Name  string
	Kind  string
	Label string
	Value any
}
