package dto

import (
	"html/template"

	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

type NoteResponse struct {
	Id                 int
	Title              string
	Content            string
	Color              string // hex sanitizado (#rgb ou #rrggbb); usar em data-note-color, nao em style com {{ }}.
	Pinned             bool
	Archived           bool
	Deleted            bool
	Tags               []NoteTagResponse
	Attachments        []NoteAttachmentResponse
	AttachmentError    string
	AttachmentRedirect string
}

type NoteTagResponse struct {
	Id       int
	Name     string
	Slug     string
	Color    string
	Selected bool
}

type NoteAttachmentResponse struct {
	Id        int
	Name      string
	MimeType  string
	SizeLabel string
	URL       string
	IsImage   bool
}

type NoteListPage struct {
	Notes           []NoteResponse
	Search          string
	Color           string
	Tag             string
	Sort            string
	PinnedOnly      bool
	ActionRedirect  string
	AvailableColors []string
	AvailableTags   []NoteTagResponse
	HasFilters      bool
	HasSearch       bool
	ResultCount     int
	ResultSummary   string
	FilterChips     []NoteFilterChip
	SearchHint      string
}

type NoteFilterChip struct {
	Label string
	Value string
	Href  string
}

type NoteRequest struct {
	Id                 int
	Title              string
	Content            string
	Color              string
	MaxTitleLen        int
	SelectedTagIDs     []int
	AvailableTags      []NoteTagResponse
	Pinned             bool
	Archived           bool
	Deleted            bool
	Attachments        []NoteAttachmentResponse
	AttachmentError    string
	AttachmentRedirect string
	CSRFField          template.HTML
	validations.FormValidator
}

type NoteTagForm struct {
	Id       int
	Name     string
	Color    string
	Redirect string
	validations.FormValidator
}

type NoteTagsPage struct {
	Tags []NoteTagResponse
	Form NoteTagForm
}
