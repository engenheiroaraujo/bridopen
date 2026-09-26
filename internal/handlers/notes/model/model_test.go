package models

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
)

// TestNoteModelZeroValues verifica valores zero e campos básicos do modelo Note.
func TestNoteModelZeroValues(t *testing.T) {
	t.Parallel()

	note := Note{
		Title: pgtype.Text{String: "Reunião", Valid: true},
		Tags:  []NoteTag{{Name: pgtype.Text{String: "Trabalho", Valid: true}}},
	}

	assert.False(t, note.Id.Valid)
	assert.True(t, note.Title.Valid)
	assert.Equal(t, "Reunião", note.Title.String)
	assert.Len(t, note.Tags, 1)
	assert.Empty(t, note.Attachments)
}
