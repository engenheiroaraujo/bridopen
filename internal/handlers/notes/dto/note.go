package dto

import (
	models "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

func NewNoteResponseFromNote(note *models.Note) (res NoteResponse) {
	res.Id = int(note.Id.Int.Int64())
	res.Title = note.Title.String
	res.Content = note.Content.String
	res.Color = sanitizeHexColor(note.Color.String)
	res.Pinned = note.Pinned.Bool
	res.Archived = note.ArchivedAt.Valid
	res.Deleted = note.DeletedAt.Valid
	res.Tags = NewNoteTagResponseList(note.Tags)
	res.Attachments = NewNoteAttachmentResponseList(note.Attachments, res.Id)
	return
}

func NewNoteResponseFromNoteList(notes []models.Note) (res []NoteResponse) {
	for _, note := range notes {
		res = append(res, NewNoteResponseFromNote(&note))
	}
	return
}

func NewNoteRequest(id int, title, content, color string, selectedTagIDs []int, availableTags []models.NoteTag) NoteRequest {
	return NoteRequest{
		Id:             id,
		Title:          title,
		Content:        content,
		Color:          sanitizeHexColor(color),
		MaxTitleLen:    validations.MaxNoteTitleLen,
		SelectedTagIDs: validations.NormalizeTagIDs(selectedTagIDs),
		AvailableTags:  NewSelectableNoteTagResponseList(availableTags, selectedTagIDs),
	}
}

func NewNoteRequestFromNote(note *models.Note, availableTags []models.NoteTag) NoteRequest {
	res := NewNoteResponseFromNote(note)
	req := NewNoteRequest(res.Id, res.Title, res.Content, res.Color, NoteTagIDsFromModels(note.Tags), availableTags)
	req.Pinned = res.Pinned
	req.Archived = res.Archived
	req.Deleted = res.Deleted
	req.Attachments = res.Attachments
	return req
}
