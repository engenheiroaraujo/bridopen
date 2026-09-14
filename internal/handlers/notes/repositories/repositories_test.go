package repositories

import (
	"context"
	"errors"
	"fmt"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
	"math/big"
	"testing"
	"time"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	return mock
}

func noteSelectCols() []string {
	return []string{"id", "title", "content", "color", "pinned", "archived_at", "deleted_at", "created_at", "updated_at"}
}

func nid(id int64) string {
	return fmt.Sprintf("%d", id)
}

func sampleNoteRow(id int64) []any {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	return []any{nid(id), "Title", "Body", "#fff", false, nil, nil, now, now}
}

func numericID(id int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(id), Valid: true}
}

func expectEmptyTagLoad(mock pgxmock.PgxPoolIface, noteID int64) {
	mock.ExpectQuery(`note_tag_links.note_id = any`).
		WithArgs([]int64{noteID}).
		WillReturnRows(pgxmock.NewRows([]string{"note_id", "id", "name", "slug", "color"}))
}

func expectEmptyAttachmentLoad(mock pgxmock.PgxPoolIface, noteID int64) {
	mock.ExpectQuery(`note_attachments.note_id = any`).
		WithArgs([]int64{noteID}).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "note_id", "user_id", "original_name", "storage_key", "mime_type", "size_bytes", "checksum_sha256", "created_at",
		}))
}

func attachmentCols() []string {
	return []string{
		"id", "note_id", "user_id", "original_name", "storage_key", "mime_type", "size_bytes", "checksum_sha256", "created_at",
	}
}

func sampleAttachmentRow(id, noteID, userID int64) []any {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	return []any{nid(id), nid(noteID), nid(userID), "file.pdf", "key/1", "application/pdf", int64(10), "abc", now}
}

// TestNormalizeTagName verifica a normalização do nome da etiqueta no repositório.
func TestNormalizeTagName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Trabalho", validations.NormalizeTagName("  #Trabalho  "))
	assert.Equal(t, "Ana Paula", validations.NormalizeTagName("#Ana   Paula"))
	assert.Equal(t, "", validations.NormalizeTagName("   #  "))
}

// TestNormalizeTagIDs verifica a remoção de IDs inválidos e duplicados.
func TestNormalizeTagIDs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []int{1, 3}, validations.NormalizeTagIDs([]int{1, 0, -2, 1, 3}))
	assert.Empty(t, validations.NormalizeTagIDs(nil))
}

// TestNormalizeNoteTagSlug verifica a geração de slug a partir do nome da etiqueta.
func TestNormalizeTagSlug(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "trabalho", validations.NormalizeTagSlug("  Trabalho  "))
	assert.Equal(t, "ana-paula", validations.NormalizeTagSlug("Ana Paula"))
	assert.Equal(t, "reunião-1", validations.NormalizeTagSlug("Reunião #1"))
	assert.Equal(t, noteTagSlug("Casa"), validations.NormalizeTagSlug("Casa"))
}

// TestNormalizeNoteColorAndNullableText verifica a normalização de cor e a conversão para texto anulável.
func TestNormalizeNoteColorAndNullableText(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "#aabbcc", validations.NormalizeHexColor("  #AABBCC  "))

	empty := nullableText("   ")
	assert.False(t, empty.Valid)

	filled := nullableText("  ok  ")
	assert.True(t, filled.Valid)
	assert.Equal(t, "ok", filled.String)
}

// TestIsUniqueViolation verifica a detecção do código PostgreSQL 23505 de violação de unicidade.
func TestIsUniqueViolation(t *testing.T) {
	t.Parallel()

	assert.True(t, isUniqueViolation(&pgconn.PgError{Code: "23505"}))
	assert.False(t, isUniqueViolation(&pgconn.PgError{Code: "23503"}))
	assert.False(t, isUniqueViolation(errors.New("other")))
}

// TestNoteOrderBySQL verifica as cláusulas ORDER BY geradas para cada modo de ordenação.
func TestNoteOrderBySQL(t *testing.T) {
	t.Parallel()

	assert.Contains(t, noteOrderBySQL(models.NoteSortRecent), "updated_at desc")
	assert.Contains(t, noteOrderBySQL(models.NoteSortOldest), "updated_at asc")
	assert.Contains(t, noteOrderBySQL(models.NoteSortPinned), "pinned desc")

	// Sem termo buscado a relevância não tem o que ranquear e cai no padrão.
	assert.Contains(t, noteOrderBySQL(models.NoteSortRelevance), "pinned desc")
}

// TestNoteRelevanceOrderBySQL verifica que a relevância prioriza o título e usa
// placeholders distintos: o padrão escapado no LIKE e o termo cru na similaridade.
func TestNoteRelevanceOrderBySQL(t *testing.T) {
	t.Parallel()

	sql := noteRelevanceOrderBySQL(2, 5)
	assert.Contains(t, sql, "notes_content.title")
	assert.Contains(t, sql, `ilike '%' || public.qn_unaccent($2) || '%' escape '\'`)
	assert.Contains(t, sql, "similarity(public.qn_unaccent(notes_content.title), public.qn_unaccent($5)) desc")
	assert.NotContains(t, sql, "pinned desc")
}

// TestNoteSearchConditionSQLUsesParam verifica que a condição de busca SQL usa o placeholder correto.
func TestNoteSearchConditionSQLUsesParam(t *testing.T) {
	t.Parallel()

	sql := noteSearchConditionSQL(4)
	assert.Contains(t, sql, "$4")
	assert.Contains(t, sql, "qn_unaccent")
	assert.Contains(t, sql, `escape '\'`)
	assert.NotContains(t, sql, fmt.Sprintf("$%d", 5))
}

// TestEscapeLikePattern verifica que os metacaracteres de LIKE são neutralizados
// para que o termo seja procurado literalmente.
func TestEscapeLikePattern(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `100\%`, escapeLikePattern("100%"))
	assert.Equal(t, `nota\_1`, escapeLikePattern("nota_1"))
	assert.Equal(t, `c:\\tmp`, escapeLikePattern(`c:\tmp`))
	assert.Equal(t, "reunião", escapeLikePattern("reunião"))
	assert.Equal(t, "", escapeLikePattern(""))
}

// TestListAttachments verifica ListAttachments em sucesso e nos caminhos de erro.
func TestListAttachments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.note_attachments`).
			WithArgs(7, 11).
			WillReturnRows(pgxmock.NewRows(attachmentCols()).AddRow(sampleAttachmentRow(1, 11, 7)...))
		list, err := repo.ListAttachments(ctx, 7, 11)
		require.NoError(t, err)
		require.Len(t, list, 1)
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.note_attachments`).WithArgs(7, 11).WillReturnError(errors.New("q"))
		_, err := repo.ListAttachments(ctx, 7, 11)
		require.Error(t, err)
	})

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.note_attachments`).
			WithArgs(7, 11).
			WillReturnRows(pgxmock.NewRows(attachmentCols()).AddRow("x", "y", "z", 1, 2, 3, "bad", 4, 5))
		_, err := repo.ListAttachments(ctx, 7, 11)
		require.Error(t, err)
	})

	t.Run("rows err", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.note_attachments`).
			WithArgs(7, 11).
			WillReturnRows(pgxmock.NewRows(attachmentCols()).
				AddRow(sampleAttachmentRow(1, 11, 7)...).
				CloseError(errors.New("r")))
		_, err := repo.ListAttachments(ctx, 7, 11)
		require.Error(t, err)
	})
}

// TestGetAttachment verifica GetAttachment em sucesso, não encontrado e erro genérico.
func TestGetAttachment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`note_attachments.id = \$1`).
			WithArgs(5, 11, 7).
			WillReturnRows(pgxmock.NewRows(attachmentCols()).AddRow(sampleAttachmentRow(5, 11, 7)...))
		att, err := repo.GetAttachment(ctx, 7, 11, 5)
		require.NoError(t, err)
		require.Equal(t, int64(5), att.Id.Int.Int64())
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`note_attachments.id = \$1`).WithArgs(5, 11, 7).WillReturnError(pgx.ErrNoRows)
		_, err := repo.GetAttachment(ctx, 7, 11, 5)
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("other error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`note_attachments.id = \$1`).WithArgs(5, 11, 7).WillReturnError(errors.New("x"))
		_, err := repo.GetAttachment(ctx, 7, 11, 5)
		require.Error(t, err)
	})
}

// TestCreateAttachment verifica CreateAttachment, incluindo limite, transação e falhas.
func TestCreateAttachment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	input := NoteAttachmentInput{
		OriginalName:   "a.pdf",
		StorageKey:     "k",
		MimeType:       "application/pdf",
		SizeBytes:      9,
		ChecksumSHA256: "sum",
	}

	t.Run("begin error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, err := repo.CreateAttachment(ctx, 7, 11, input)
		require.Error(t, err)
	})

	t.Run("note not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`for update`).WithArgs(11, 7).WillReturnError(pgx.ErrNoRows)
		mock.ExpectRollback()
		_, err := repo.CreateAttachment(ctx, 7, 11, input)
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("lock error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`for update`).WithArgs(11, 7).WillReturnError(errors.New("lock"))
		mock.ExpectRollback()
		_, err := repo.CreateAttachment(ctx, 7, 11, input)
		require.Error(t, err)
	})

	t.Run("count error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`for update`).
			WithArgs(11, 7).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(11)))
		mock.ExpectQuery(`select count`).WithArgs(11, 7).WillReturnError(errors.New("count"))
		mock.ExpectRollback()
		_, err := repo.CreateAttachment(ctx, 7, 11, input)
		require.Error(t, err)
	})

	t.Run("limit reached", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`for update`).
			WithArgs(11, 7).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(11)))
		mock.ExpectQuery(`select count`).
			WithArgs(11, 7).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(validations.MaxNoteAttachmentsPerNote))
		mock.ExpectRollback()
		_, err := repo.CreateAttachment(ctx, 7, 11, input)
		require.ErrorIs(t, err, apperrors.ErrAttachmentLimit)
	})

	t.Run("insert error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`for update`).
			WithArgs(11, 7).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(11)))
		mock.ExpectQuery(`select count`).
			WithArgs(11, 7).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`insert into public.note_attachments`).
			WithArgs(11, 7, input.OriginalName, input.StorageKey, input.MimeType, input.SizeBytes, pgxmock.AnyArg()).
			WillReturnError(errors.New("ins"))
		mock.ExpectRollback()
		_, err := repo.CreateAttachment(ctx, 7, 11, input)
		require.Error(t, err)
	})

	t.Run("commit error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`for update`).
			WithArgs(11, 7).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(11)))
		mock.ExpectQuery(`select count`).
			WithArgs(11, 7).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`insert into public.note_attachments`).
			WithArgs(11, 7, input.OriginalName, input.StorageKey, input.MimeType, input.SizeBytes, pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows(attachmentCols()).AddRow(sampleAttachmentRow(9, 11, 7)...))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, err := repo.CreateAttachment(ctx, 7, 11, input)
		require.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`for update`).
			WithArgs(11, 7).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(11)))
		mock.ExpectQuery(`select count`).
			WithArgs(11, 7).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`insert into public.note_attachments`).
			WithArgs(11, 7, input.OriginalName, input.StorageKey, input.MimeType, input.SizeBytes, pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows(attachmentCols()).AddRow(sampleAttachmentRow(9, 11, 7)...))
		mock.ExpectCommit()
		att, err := repo.CreateAttachment(ctx, 7, 11, input)
		require.NoError(t, err)
		require.Equal(t, int64(9), att.Id.Int.Int64())
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestDeleteAttachment verifica DeleteAttachment em sucesso, não encontrado e erro.
func TestDeleteAttachment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`delete from public.note_attachments`).
			WithArgs(5, 11, 7).
			WillReturnRows(pgxmock.NewRows(attachmentCols()).AddRow(sampleAttachmentRow(5, 11, 7)...))
		att, err := repo.DeleteAttachment(ctx, 7, 11, 5)
		require.NoError(t, err)
		require.NotNil(t, att)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`delete from public.note_attachments`).WithArgs(5, 11, 7).WillReturnError(pgx.ErrNoRows)
		_, err := repo.DeleteAttachment(ctx, 7, 11, 5)
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`delete from public.note_attachments`).WithArgs(5, 11, 7).WillReturnError(errors.New("x"))
		_, err := repo.DeleteAttachment(ctx, 7, 11, 5)
		require.Error(t, err)
	})
}

// TestLoadAttachmentsForNotesEdges verifica loadAttachmentsForNotes em entradas vazias e erros de carga.
func TestLoadAttachmentsForNotesEdges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := &NoteRepository{}

	notes, err := repo.loadAttachmentsForNotes(ctx, nil)
	require.NoError(t, err)
	require.Nil(t, notes)

	notes, err = repo.loadAttachmentsForNotes(ctx, []models.Note{{}})
	require.NoError(t, err)
	require.Len(t, notes, 1)

	mock := newMockPool(t)
	repo = NewNoteRepository(mock)
	mock.ExpectQuery(`note_attachments.note_id = any`).
		WithArgs([]int64{9}).
		WillReturnRows(pgxmock.NewRows(attachmentCols()).AddRow("bad", 1, 2, 3, 4, 5, 6, 7, 8))
	_, err = repo.loadAttachmentsForNotes(ctx, []models.Note{{Id: pgtype.Numeric{Int: big.NewInt(9), Valid: true}}})
	require.Error(t, err)

	mock = newMockPool(t)
	repo = NewNoteRepository(mock)
	mock.ExpectQuery(`note_attachments.note_id = any`).
		WithArgs([]int64{9}).
		WillReturnRows(pgxmock.NewRows(attachmentCols()).
			AddRow(sampleAttachmentRow(1, 9, 7)...).
			CloseError(errors.New("r")))
	_, err = repo.loadAttachmentsForNotes(ctx, []models.Note{{Id: pgtype.Numeric{Int: big.NewInt(9), Valid: true}}})
	require.Error(t, err)

	// anexo com NoteId nil é omitido do mapa; nota com Id nil é omitida na atribuição
	mock = newMockPool(t)
	repo = NewNoteRepository(mock)
	row := sampleAttachmentRow(1, 9, 7)
	row[1] = nil // note_id
	mock.ExpectQuery(`note_attachments.note_id = any`).
		WithArgs([]int64{9}).
		WillReturnRows(pgxmock.NewRows(attachmentCols()).AddRow(row...))
	out, err := repo.loadAttachmentsForNotes(ctx, []models.Note{
		{Id: pgtype.Numeric{Int: big.NewInt(9), Valid: true}},
		{},
	})
	require.NoError(t, err)
	require.Empty(t, out[0].Attachments)
}

// TestGetById verifica GetById nos caminhos de sucesso, não encontrado e erros de carga.
func TestGetById(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectQuery(`from\s+public.notes`).
			WithArgs(7, 11).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow(sampleNoteRow(11)...))
		expectEmptyTagLoad(mock, 11)
		expectEmptyAttachmentLoad(mock, 11)

		note, err := repo.GetById(ctx, 7, 11)
		require.NoError(t, err)
		require.Equal(t, int64(11), note.Id.Int.Int64())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.notes`).WithArgs(7, 11).WillReturnError(pgx.ErrNoRows)
		_, err := repo.GetById(ctx, 7, 11)
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.notes`).WithArgs(7, 11).WillReturnError(errors.New("db"))
		_, err := repo.GetById(ctx, 7, 11)
		require.Error(t, err)
	})

	t.Run("tag load error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.notes`).
			WithArgs(7, 11).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow(sampleNoteRow(11)...))
		mock.ExpectQuery(`note_tag_links.note_id = any`).
			WithArgs([]int64{11}).
			WillReturnError(errors.New("tags"))
		_, err := repo.GetById(ctx, 7, 11)
		require.Error(t, err)
	})

	t.Run("attachment load error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.notes`).
			WithArgs(7, 11).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow(sampleNoteRow(11)...))
		expectEmptyTagLoad(mock, 11)
		mock.ExpectQuery(`note_attachments.note_id = any`).
			WithArgs([]int64{11}).
			WillReturnError(errors.New("att"))
		_, err := repo.GetById(ctx, 7, 11)
		require.Error(t, err)
	})

	t.Run("loads tags and attachments", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.notes`).
			WithArgs(7, 11).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow(sampleNoteRow(11)...))
		mock.ExpectQuery(`note_tag_links.note_id = any`).
			WithArgs([]int64{11}).
			WillReturnRows(pgxmock.NewRows([]string{"note_id", "id", "name", "slug", "color"}).
				AddRow(int64(11), "2", "Work", "work", "#fff"))
		mock.ExpectQuery(`note_attachments.note_id = any`).
			WithArgs([]int64{11}).
			WillReturnRows(pgxmock.NewRows(attachmentCols()).AddRow(sampleAttachmentRow(5, 11, 7)...))

		note, err := repo.GetById(ctx, 7, 11)
		require.NoError(t, err)
		require.Len(t, note.Tags, 1)
		require.Len(t, note.Attachments, 1)
	})
}

// TestCreateNote verifica Create de nota, incluindo sync de tags e falhas de transação.
func TestCreateNote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("begin error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, err := repo.Create(ctx, 7, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("insert note error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.notes`).WithArgs(7).WillReturnError(errors.New("ins"))
		mock.ExpectRollback()
		_, err := repo.Create(ctx, 7, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("insert content error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow("11", timeNow(), timeNow()))
		mock.ExpectExec(`insert into public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(errors.New("content"))
		mock.ExpectRollback()
		_, err := repo.Create(ctx, 7, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("commit error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow("11", timeNow(), timeNow()))
		mock.ExpectExec(`insert into public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectExec(`delete from public.note_tag_links`).
			WithArgs(int64(11), 7).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, err := repo.Create(ctx, 7, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("skips sync when id invalid", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		// id nulo -> note.Id.Int == nil
		mock.ExpectQuery(`insert into public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow(nil, timeNow(), timeNow()))
		mock.ExpectExec(`insert into public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		_, err := repo.Create(ctx, 7, "t", "c", "#fff", []int{1})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestUpdateNote verifica Update de nota nos caminhos de sucesso, não encontrado e erro.
func TestUpdateNote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.notes`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`delete from public.note_tag_links`).
			WithArgs(int64(11), 7).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectCommit()

		note, err := repo.Update(ctx, 7, 11, "t", "c", "#fff", nil)
		require.NoError(t, err)
		require.NotNil(t, note)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin().WillReturnError(errors.New("begin"))
		_, err := repo.Update(ctx, 7, 11, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("notes update error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.notes`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnError(errors.New("upd"))
		mock.ExpectRollback()
		_, err := repo.Update(ctx, 7, 11, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("notes not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.notes`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectRollback()
		_, err := repo.Update(ctx, 7, 11, "t", "c", "#fff", nil)
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("content update error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.notes`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnError(errors.New("content"))
		mock.ExpectRollback()
		_, err := repo.Update(ctx, 7, 11, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("content not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.notes`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectRollback()
		_, err := repo.Update(ctx, 7, 11, "t", "c", "#fff", nil)
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("sync error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.notes`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`delete from public.note_tag_links`).
			WithArgs(int64(11), 7).
			WillReturnError(errors.New("sync"))
		mock.ExpectRollback()
		_, err := repo.Update(ctx, 7, 11, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("commit error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.notes`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`delete from public.note_tag_links`).
			WithArgs(int64(11), 7).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectCommit().WillReturnError(errors.New("commit"))
		mock.ExpectRollback()
		_, err := repo.Update(ctx, 7, 11, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("with tags", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectBegin()
		mock.ExpectExec(`update public.notes`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`update public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`tag_id <> all`).
			WithArgs(int64(11), 7, []int64{3}).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectExec(`insert into public.note_tag_links`).
			WithArgs(int64(11), 7, []int64{3}).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		_, err := repo.Update(ctx, 7, 11, "t", "c", "#fff", []int{3})
		require.NoError(t, err)
	})
}

// TestListNotes verifica List de notas com filtros, ordenações e erros de consulta.
func TestListNotes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success with filters", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectQuery(`qn_unaccent`).
			WithArgs(7, "hello", "#abc", "work").
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow(sampleNoteRow(11)...))
		expectEmptyTagLoad(mock, 11)

		list, err := repo.List(ctx, 7, models.NoteFilter{
			Search: " hello ",
			Color:  " #ABC ",
			Tag:    " work ",
			Sort:   models.NoteSortRecent,
			Pinned: true,
		})
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("sort oldest", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`updated_at asc`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()))
		list, err := repo.List(ctx, 7, models.NoteFilter{Sort: models.NoteSortOldest})
		require.NoError(t, err)
		assert.Empty(t, list)
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.notes`).WithArgs(7).WillReturnError(errors.New("q"))
		_, err := repo.List(ctx, 7, models.NoteFilter{})
		require.Error(t, err)
	})

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow("bad", 1, 2, 3, 4, 5, 6, 7, 8))
		_, err := repo.List(ctx, 7, models.NoteFilter{})
		require.Error(t, err)
	})

	t.Run("rows err", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).
				AddRow(sampleNoteRow(11)...).
				CloseError(errors.New("row")))
		_, err := repo.List(ctx, 7, models.NoteFilter{})
		require.Error(t, err)
	})
}

// TestListColors verifica ListColors de cores distintas e os caminhos de erro.
func TestListColors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`select distinct`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"color"}).AddRow("#fff").AddRow("#000"))
		list, err := repo.ListColors(ctx, 7)
		require.NoError(t, err)
		assert.Equal(t, []string{"#fff", "#000"}, list)
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`select distinct`).WithArgs(7).WillReturnError(errors.New("q"))
		_, err := repo.ListColors(ctx, 7)
		require.Error(t, err)
	})

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`select distinct`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"color"}).AddRow(make(chan int)))
		_, err := repo.ListColors(ctx, 7)
		require.Error(t, err)
	})

	t.Run("rows err", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`select distinct`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"color"}).AddRow("#fff").CloseError(errors.New("r")))
		_, err := repo.ListColors(ctx, 7)
		require.Error(t, err)
	})
}

// TestListArchivedAndDeleted verifica ListArchived e ListDeleted em sucesso e erro.
func TestListArchivedAndDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("archived success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`archived_at is not null`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow(sampleNoteRow(3)...))
		expectEmptyTagLoad(mock, 3)
		list, err := repo.ListArchived(ctx, 7)
		require.NoError(t, err)
		require.Len(t, list, 1)
	})

	t.Run("archived query/scan/rows errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`archived_at is not null`).WithArgs(7).WillReturnError(errors.New("q"))
		_, err := repo.ListArchived(ctx, 7)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewNoteRepository(mock)
		mock.ExpectQuery(`archived_at is not null`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow("x", 1, 2, 3, 4, 5, 6, 7, 8))
		_, err = repo.ListArchived(ctx, 7)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewNoteRepository(mock)
		mock.ExpectQuery(`archived_at is not null`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow(sampleNoteRow(1)...).CloseError(errors.New("r")))
		_, err = repo.ListArchived(ctx, 7)
		require.Error(t, err)
	})

	t.Run("deleted success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`deleted_at is not null`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow(sampleNoteRow(4)...))
		expectEmptyTagLoad(mock, 4)
		list, err := repo.ListDeleted(ctx, 7)
		require.NoError(t, err)
		require.Len(t, list, 1)
	})

	t.Run("deleted query/scan/rows errors", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`deleted_at is not null`).WithArgs(7).WillReturnError(errors.New("q"))
		_, err := repo.ListDeleted(ctx, 7)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewNoteRepository(mock)
		mock.ExpectQuery(`deleted_at is not null`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow("x", 1, 2, 3, 4, 5, 6, 7, 8))
		_, err = repo.ListDeleted(ctx, 7)
		require.Error(t, err)

		mock = newMockPool(t)
		repo = NewNoteRepository(mock)
		mock.ExpectQuery(`deleted_at is not null`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows(noteSelectCols()).AddRow(sampleNoteRow(1)...).CloseError(errors.New("r")))
		_, err = repo.ListDeleted(ctx, 7)
		require.Error(t, err)
	})
}

// TestLoadTagsForNotesEdges verifica loadTagsForNotes em entradas vazias, IDs inválidos e erros de scan.
func TestLoadTagsForNotesEdges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := &NoteRepository{}

	notes, err := repo.loadTagsForNotes(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, notes)

	notes, err = repo.loadTagsForNotes(ctx, []models.Note{{}})
	require.NoError(t, err)
	require.Len(t, notes, 1)

	mock := newMockPool(t)
	repo = NewNoteRepository(mock)
	withID := []models.Note{{Id: pgtype.Numeric{Int: big.NewInt(9), Valid: true}}, {}}
	mock.ExpectQuery(`note_tag_links.note_id = any`).
		WithArgs([]int64{9}).
		WillReturnRows(pgxmock.NewRows([]string{"note_id", "id", "name", "slug", "color"}).
			AddRow("bad", 1, 2, 3, 4))
	_, err = repo.loadTagsForNotes(ctx, withID)
	require.Error(t, err)

	mock = newMockPool(t)
	repo = NewNoteRepository(mock)
	mock.ExpectQuery(`note_tag_links.note_id = any`).
		WithArgs([]int64{9}).
		WillReturnRows(pgxmock.NewRows([]string{"note_id", "id", "name", "slug", "color"}).
			AddRow(int64(9), "1", "A", "a", "#f").
			CloseError(errors.New("r")))
	_, err = repo.loadTagsForNotes(ctx, []models.Note{{Id: pgtype.Numeric{Int: big.NewInt(9), Valid: true}}})
	require.Error(t, err)

	// o loop de atribuição ignora notas sem id numérico
	mock = newMockPool(t)
	repo = NewNoteRepository(mock)
	mock.ExpectQuery(`note_tag_links.note_id = any`).
		WithArgs([]int64{9}).
		WillReturnRows(pgxmock.NewRows([]string{"note_id", "id", "name", "slug", "color"}).
			AddRow(int64(9), "1", "A", "a", "#f"))
	out, err := repo.loadTagsForNotes(ctx, []models.Note{
		{Id: pgtype.Numeric{Int: big.NewInt(9), Valid: true}},
		{},
	})
	require.NoError(t, err)
	require.Len(t, out[0].Tags, 1)
	require.Nil(t, out[1].Tags)
}

// TestExecNoteStatePaths verifica execNoteState nos caminhos de sucesso, não encontrado e erro de exec.
func TestExecNoteStatePaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectExec(`update public.notes`).
			WithArgs(true, 1, 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		require.NoError(t, repo.SetPinned(ctx, 7, 1, true))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectExec(`update public.notes`).
			WithArgs(1, 7).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err := repo.Archive(ctx, 7, 1)
		require.ErrorIs(t, err, apperrors.ErrNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exec error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectExec(`update public.notes`).
			WithArgs(1, 7).
			WillReturnError(errors.New("db down"))

		err := repo.Unarchive(ctx, 7, 1)
		require.Error(t, err)
		var repoErr *apperrors.RepositoryError
		require.ErrorAs(t, err, &repoErr)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestNoteStateMethods verifica MoveToTrash, Restore, DeletePermanently e SetPinned via SQL esperado.
func TestNoteStateMethods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cases := []struct {
		name string
		sql  string
		call func(repo *NoteRepository) error
		args []any
	}{
		{
			name: "move to trash",
			sql:  `deleted_at = current_timestamp`,
			call: func(repo *NoteRepository) error { return repo.MoveToTrash(ctx, 7, 3) },
			args: []any{3, 7},
		},
		{
			name: "restore",
			sql:  `deleted_at = null`,
			call: func(repo *NoteRepository) error { return repo.Restore(ctx, 7, 3) },
			args: []any{3, 7},
		},
		{
			name: "delete permanently",
			sql:  `delete from public.notes`,
			call: func(repo *NoteRepository) error { return repo.DeletePermanently(ctx, 7, 3) },
			args: []any{3, 7},
		},
		{
			name: "delete aliases trash",
			sql:  `deleted_at = current_timestamp`,
			call: func(repo *NoteRepository) error { return repo.Delete(ctx, 7, 3) },
			args: []any{3, 7},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mock := newMockPool(t)
			repo := NewNoteRepository(mock)
			mock.ExpectExec(tc.sql).
				WithArgs(tc.args...).
				WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			assert.NoError(t, tc.call(repo))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestListTags verifica ListTags em sucesso e nos caminhos de erro de query/scan.
func TestListTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectQuery(`from\s+public.note_tags`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "slug", "color"}).
				AddRow("1", "Work", "work", "#fff"))

		list, err := repo.ListTags(ctx, 7)
		require.NoError(t, err)
		require.Len(t, list, 1)
		assert.Equal(t, "Work", list[0].Name.String)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.note_tags`).WithArgs(7).WillReturnError(errors.New("q"))
		_, err := repo.ListTags(ctx, 7)
		require.Error(t, err)
	})

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.note_tags`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "slug", "color"}).AddRow("bad", 1, 2, 3))
		_, err := repo.ListTags(ctx, 7)
		require.Error(t, err)
	})

	t.Run("rows err", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`from\s+public.note_tags`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "slug", "color"}).
				AddRow("1", "Work", "work", "#fff").
				CloseError(errors.New("row boom")))
		_, err := repo.ListTags(ctx, 7)
		require.Error(t, err)
	})
}

// TestCreateTag verifica CreateTag com nome vazio, sucesso e violação de unicidade.
func TestCreateTag(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("empty name", func(t *testing.T) {
		t.Parallel()
		repo := NewNoteRepository(newMockPool(t))
		_, err := repo.CreateTag(ctx, 7, "   #  ", "#fff")
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`insert into public.note_tags`).
			WithArgs(7, "Work", "work", pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "slug", "color"}).
				AddRow("2", "Work", "work", "#abc"))
		tag, err := repo.CreateTag(ctx, 7, "Work", "#ABC")
		require.NoError(t, err)
		assert.Equal(t, "Work", tag.Name.String)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unique violation", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`insert into public.note_tags`).
			WithArgs(7, "Work", "work", pgxmock.AnyArg()).
			WillReturnError(&pgconn.PgError{Code: "23505"})
		_, err := repo.CreateTag(ctx, 7, "Work", "")
		require.ErrorIs(t, err, apperrors.ErrDuplicate)
	})

	t.Run("other error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`insert into public.note_tags`).
			WithArgs(7, "Work", "work", pgxmock.AnyArg()).
			WillReturnError(errors.New("fail"))
		_, err := repo.CreateTag(ctx, 7, "Work", "")
		require.Error(t, err)
	})
}

// TestUpdateTag verifica UpdateTag em sucesso, não encontrado, duplicata e erros.
func TestUpdateTag(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("empty name", func(t *testing.T) {
		t.Parallel()
		_, err := NewNoteRepository(newMockPool(t)).UpdateTag(ctx, 7, 1, "   ", "")
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`update public.note_tags`).
			WithArgs("Home", "home", pgxmock.AnyArg(), 1, 7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "slug", "color"}).
				AddRow("1", "Home", "home", "#000"))
		tag, err := repo.UpdateTag(ctx, 7, 1, "Home", "#000")
		require.NoError(t, err)
		assert.Equal(t, "Home", tag.Name.String)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`update public.note_tags`).
			WithArgs("Home", "home", pgxmock.AnyArg(), 1, 7).
			WillReturnError(pgx.ErrNoRows)
		_, err := repo.UpdateTag(ctx, 7, 1, "Home", "")
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`update public.note_tags`).
			WithArgs("Home", "home", pgxmock.AnyArg(), 1, 7).
			WillReturnError(&pgconn.PgError{Code: "23505"})
		_, err := repo.UpdateTag(ctx, 7, 1, "Home", "")
		require.ErrorIs(t, err, apperrors.ErrDuplicate)
	})

	t.Run("other error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectQuery(`update public.note_tags`).
			WithArgs("Home", "home", pgxmock.AnyArg(), 1, 7).
			WillReturnError(errors.New("boom"))
		_, err := repo.UpdateTag(ctx, 7, 1, "Home", "")
		require.Error(t, err)
	})
}

// TestDeleteTag verifica DeleteTag em sucesso, não encontrado e erro.
func TestDeleteTag(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectExec(`delete from public.note_tags`).
			WithArgs(1, 7).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))
		require.NoError(t, repo.DeleteTag(ctx, 7, 1))
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectExec(`delete from public.note_tags`).
			WithArgs(1, 7).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		require.ErrorIs(t, repo.DeleteTag(ctx, 7, 1), apperrors.ErrNotFound)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)
		mock.ExpectExec(`delete from public.note_tags`).
			WithArgs(1, 7).
			WillReturnError(errors.New("x"))
		require.Error(t, repo.DeleteTag(ctx, 7, 1))
	})
}

// TestSyncNoteTagsViaCreate verifica a sincronização de tags da nota durante Create.
func TestSyncNoteTagsViaCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("clear tags", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow("11", timeNow(), timeNow()))
		mock.ExpectExec(`insert into public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectExec(`delete from public.note_tag_links`).
			WithArgs(int64(11), 7).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectCommit()

		note, err := repo.Create(ctx, 7, "t", "c", "#fff", nil)
		require.NoError(t, err)
		require.NotNil(t, note)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("assign tags", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow("11", timeNow(), timeNow()))
		mock.ExpectExec(`insert into public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectExec(`tag_id <> all`).
			WithArgs(int64(11), 7, []int64{1, 2}).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectExec(`insert into public.note_tag_links`).
			WithArgs(int64(11), 7, []int64{1, 2}).
			WillReturnResult(pgxmock.NewResult("INSERT", 2))
		mock.ExpectCommit()

		_, err := repo.Create(ctx, 7, "t", "c", "#fff", []int{1, 2, 2})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("clear tags error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow("11", timeNow(), timeNow()))
		mock.ExpectExec(`insert into public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectExec(`delete from public.note_tag_links`).
			WithArgs(int64(11), 7).
			WillReturnError(errors.New("sync fail"))
		mock.ExpectRollback()

		_, err := repo.Create(ctx, 7, "t", "c", "#fff", nil)
		require.Error(t, err)
	})

	t.Run("insert links error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow("11", timeNow(), timeNow()))
		mock.ExpectExec(`insert into public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectExec(`tag_id <> all`).
			WithArgs(int64(11), 7, []int64{1}).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
		mock.ExpectExec(`insert into public.note_tag_links`).
			WithArgs(int64(11), 7, []int64{1}).
			WillReturnError(errors.New("link fail"))
		mock.ExpectRollback()

		_, err := repo.Create(ctx, 7, "t", "c", "#fff", []int{1})
		require.Error(t, err)
	})

	t.Run("delete filtered tags error", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		repo := NewNoteRepository(mock)

		mock.ExpectBegin()
		mock.ExpectQuery(`insert into public.notes`).
			WithArgs(7).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow("11", timeNow(), timeNow()))
		mock.ExpectExec(`insert into public.notes_content`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectExec(`tag_id <> all`).
			WithArgs(int64(11), 7, []int64{1}).
			WillReturnError(errors.New("del fail"))
		mock.ExpectRollback()

		_, err := repo.Create(ctx, 7, "t", "c", "#fff", []int{1})
		require.Error(t, err)
	})
}

func timeNow() time.Time {
	return time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
}
