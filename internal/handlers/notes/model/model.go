package models

import (
	"github.com/jackc/pgx/v5/pgtype"
)

type Note struct {
	Id          pgtype.Numeric
	Title       pgtype.Text
	Content     pgtype.Text
	Color       pgtype.Text
	Pinned      pgtype.Bool
	ArchivedAt  pgtype.Timestamp
	DeletedAt   pgtype.Timestamp
	CreatedAt   pgtype.Timestamp
	UpdatedAt   pgtype.Timestamp
	Tags        []NoteTag
	Attachments []NoteAttachment
}

type NoteTag struct {
	Id    pgtype.Numeric
	Name  pgtype.Text
	Slug  pgtype.Text
	Color pgtype.Text
}

type NoteAttachment struct {
	Id             pgtype.Numeric
	NoteId         pgtype.Numeric
	UserId         pgtype.Numeric
	OriginalName   pgtype.Text
	StorageKey     pgtype.Text
	MimeType       pgtype.Text
	SizeBytes      pgtype.Int8
	ChecksumSHA256 pgtype.Text
	CreatedAt      pgtype.Timestamp
}

// Modos de ordenação aceitos na listagem de notas.
const (
	NoteSortRecent    = "recent"
	NoteSortOldest    = "oldest"
	NoteSortPinned    = "pinned"
	NoteSortRelevance = "relevance"
)

// NoteFilter são os critérios de busca da listagem. Vive no model para que o
// handler possa montá-lo e o repositório consumi-lo sem que nenhum dos dois
// precise conhecer o outro.
type NoteFilter struct {
	Search string
	Color  string
	Tag    string
	Sort   string
	Pinned bool
}
