package main

import (
	"math/big"
	"strings"
	"time"

	notemodel "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	userdto "github.com/engenheiroaraujo/bridopen/internal/handlers/users/dto"
	"github.com/jackc/pgx/v5/pgtype"
)

// Dados fixos das telas do app. Os ids são estáveis para os testes poderem
// apontar para eles: 1 a 3 ativas, 4 arquivada, 5 na lixeira.
const (
	noteArchivedID = 4
	noteDeletedID  = 5
)

var fixtureTime = time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)

// tinyPNG é um PNG 1x1 transparente, servido como miniatura de anexo.
var tinyPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x60, 0x00, 0x02, 0x00,
	0x00, 0x05, 0x00, 0x01, 0xE2, 0x26, 0x05, 0x9B, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
	0xAE, 0x42, 0x60, 0x82,
}

func num(v int64) pgtype.Numeric { return pgtype.Numeric{Int: big.NewInt(v), Valid: true} }
func text(s string) pgtype.Text  { return pgtype.Text{String: s, Valid: true} }
func boolean(b bool) pgtype.Bool { return pgtype.Bool{Bool: b, Valid: true} }
func stamp(valid bool) pgtype.Timestamp {
	return pgtype.Timestamp{Time: fixtureTime, Valid: valid}
}

var fixtureTags = []notemodel.NoteTag{
	{Id: num(1), Name: text("Trabalho"), Slug: text("trabalho"), Color: text("#2563eb")},
	{Id: num(2), Name: text("Pessoal"), Slug: text("pessoal"), Color: text("#16a34a")},
	{Id: num(3), Name: text("Ideias"), Slug: text("ideias"), Color: text("")},
}

var fixtureColors = []string{"#fef3c7", "#dbeafe", "#dcfce7"}

func fixtureAttachments(noteID int64) []notemodel.NoteAttachment {
	return []notemodel.NoteAttachment{
		{Id: num(10), NoteId: num(noteID), OriginalName: text("planta.png"), MimeType: text("image/png"), SizeBytes: pgtype.Int8{Int64: 245_760, Valid: true}},
		{Id: num(11), NoteId: num(noteID), OriginalName: text("contrato.pdf"), MimeType: text("application/pdf"), SizeBytes: pgtype.Int8{Int64: 1_258_291, Valid: true}},
	}
}

func fixtureNotes() []notemodel.Note {
	return []notemodel.Note{
		{
			Id: num(1), Title: text("Reunião de planejamento"), Color: text("#fef3c7"), Pinned: boolean(true),
			Content:     text("Definir metas do trimestre e revisar o orçamento."),
			Tags:        []notemodel.NoteTag{fixtureTags[0]},
			Attachments: fixtureAttachments(1),
			CreatedAt:   stamp(true), UpdatedAt: stamp(true),
		},
		{
			Id: num(2), Title: text("Lista de compras"), Color: text("#dbeafe"), Pinned: boolean(false),
			Content:   text("Café, pão integral, frutas e leite."),
			Tags:      []notemodel.NoteTag{fixtureTags[1]},
			CreatedAt: stamp(true), UpdatedAt: stamp(true),
		},
		{
			Id: num(3), Title: text(""), Color: text("#dcfce7"), Pinned: boolean(false),
			Content:   text("Rascunho sem título para testar o fallback."),
			CreatedAt: stamp(true), UpdatedAt: stamp(true),
		},
		{
			Id: num(noteArchivedID), Title: text("Relatório antigo"), Color: text("#fef3c7"), Pinned: boolean(false),
			Content:    text("Nota arquivada no fim do semestre."),
			Tags:       []notemodel.NoteTag{fixtureTags[0], fixtureTags[2]},
			ArchivedAt: stamp(true), CreatedAt: stamp(true), UpdatedAt: stamp(true),
		},
		{
			Id: num(noteDeletedID), Title: text("Ideia descartada"), Color: text("#dbeafe"), Pinned: boolean(false),
			Content:   text("Nota que foi para a lixeira."),
			DeletedAt: stamp(true), CreatedAt: stamp(true), UpdatedAt: stamp(true),
		},
	}
}

func fixtureNoteByID(id int64) *notemodel.Note {
	for _, note := range fixtureNotes() {
		if note.Id.Int.Int64() == id {
			n := note
			return &n
		}
	}
	return nil
}

func fixtureActiveNotes(filter notemodel.NoteFilter) []notemodel.Note {
	var out []notemodel.Note
	for _, note := range fixtureNotes() {
		if note.ArchivedAt.Valid || note.DeletedAt.Valid {
			continue
		}
		if filter.Pinned && !note.Pinned.Bool {
			continue
		}
		if filter.Color != "" && !strings.EqualFold(note.Color.String, filter.Color) {
			continue
		}
		if filter.Tag != "" && !noteHasTag(note, filter.Tag) {
			continue
		}
		if filter.Search != "" && !noteMatches(note, filter.Search) {
			continue
		}
		out = append(out, note)
	}
	return out
}

func noteHasTag(note notemodel.Note, slug string) bool {
	for _, tag := range note.Tags {
		if tag.Slug.String == slug {
			return true
		}
	}
	return false
}

func noteMatches(note notemodel.Note, search string) bool {
	search = strings.ToLower(search)
	return strings.Contains(strings.ToLower(note.Title.String), search) ||
		strings.Contains(strings.ToLower(note.Content.String), search)
}

func fixtureNotesWhere(archived, deleted bool) []notemodel.Note {
	var out []notemodel.Note
	for _, note := range fixtureNotes() {
		if note.ArchivedAt.Valid == archived && note.DeletedAt.Valid == deleted && (archived || deleted) {
			out = append(out, note)
		}
	}
	return out
}

func fixtureSessions(count int) []userdto.ActiveSessionResponse {
	all := []userdto.ActiveSessionResponse{
		{ID: 1, SessionID: "sess-atual", Device: "Windows", Browser: "Chrome 140", IPAddress: "192.168.0.10", CreatedAt: "01/09/2026 10:00", LastSeenAt: "16/09/2026 18:00", Current: true},
		{ID: 2, SessionID: "sess-celular", Device: "Android", Browser: "Chrome Mobile", IPAddress: "10.0.0.5", CreatedAt: "05/09/2026 08:30", LastSeenAt: "15/09/2026 22:10"},
		{ID: 3, SessionID: "sess-notebook", Device: "macOS", Browser: "Safari 18", IPAddress: "172.16.4.2", CreatedAt: "10/09/2026 14:00", LastSeenAt: "12/09/2026 09:45"},
	}
	if count < 0 {
		count = 0
	}
	if count > len(all) {
		count = len(all)
	}
	return all[:count]
}
