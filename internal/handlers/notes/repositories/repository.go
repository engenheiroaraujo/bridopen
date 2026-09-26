package repositories

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	"github.com/engenheiroaraujo/bridopen/internal/platform/database"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type NoteAttachmentInput struct {
	OriginalName   string
	StorageKey     string
	MimeType       string
	SizeBytes      int64
	ChecksumSHA256 string
}

// NoteRepository concentra o acesso a dados de notas, etiquetas e anexos.
// O contrato consumido pela camada de serviço é declarado lá, no pacote
// service, não aqui.
type NoteRepository struct {
	db database.Pool
}

func NewNoteRepository(db database.Pool) *NoteRepository {
	return &NoteRepository{db: db}
}

// -----------------------------------------------------------------------------
// CRUD
// -----------------------------------------------------------------------------

func (nr *NoteRepository) GetById(ctx context.Context, userId, id int) (*models.Note, error) {
	var note models.Note

	query := `
		select
			notes.id
		,	notes_content.title
		,	notes_content.content
		,	notes_content.color
		,	notes.pinned
		,	notes.archived_at
		,	notes.deleted_at
		,	notes.created_at
		,	notes.updated_at
		from
			public.notes
			inner join public.notes_content
				on notes.id = notes_content.note_id
		where 1=1
			and notes.user_id = $1
			and notes.id = $2
		order by
			notes.id;
	`
	row := nr.db.QueryRow(ctx, query, userId, id)
	err := row.Scan(&note.Id, &note.Title, &note.Content, &note.Color, &note.Pinned, &note.ArchivedAt, &note.DeletedAt, &note.CreatedAt, &note.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	notes, err := nr.loadTagsForNotes(ctx, []models.Note{note})
	if err != nil {
		return nil, err
	}
	notes, err = nr.loadAttachmentsForNotes(ctx, notes)
	if err != nil {
		return nil, err
	}
	return &notes[0], nil
}

/*verificar transação*/
func (nr *NoteRepository) Create(ctx context.Context, userId int, title, content, color string, tagIDs []int) (*models.Note, error) {
	var note models.Note
	note.Title = pgtype.Text{String: title, Valid: true}
	note.Content = pgtype.Text{String: content, Valid: true}
	note.Color = pgtype.Text{String: validations.NormalizeHexColor(color), Valid: true}

	tx, err := nr.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return &note, apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	query := `
		insert into public.notes (user_id) values($1) returning id, created_at, updated_at;
	`
	row := tx.QueryRow(ctx, query, userId)
	if err := row.Scan(&note.Id, &note.CreatedAt, &note.UpdatedAt); err != nil {
		return &note, apperrors.NewRepositoryError(err)
	}

	query = `
		insert into public.notes_content (note_id, title, content, color) values($1, $2, $3, $4);
	`
	_, err = tx.Exec(ctx, query, &note.Id, note.Title, note.Content, note.Color)
	if err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}

	if note.Id.Int != nil {
		if err := nr.syncNoteTags(ctx, tx, userId, note.Id.Int.Int64(), tagIDs); err != nil {
			return &note, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return &note, apperrors.NewRepositoryError(err)
	}

	return &note, nil
}

func (nr *NoteRepository) Update(ctx context.Context, userId, id int, title, content, color string, tagIDs []int) (*models.Note, error) {
	var note models.Note

	note.Id = pgtype.Numeric{Int: big.NewInt(int64(id)), Valid: true}
	// sempre valid: true com string (mesmo ""); pgtype zero value gera null e quebra not null no postgres.
	note.Title = pgtype.Text{String: strings.TrimSpace(title), Valid: true}
	note.Content = pgtype.Text{String: content, Valid: true}
	note.Color = pgtype.Text{String: validations.NormalizeHexColor(color), Valid: true}
	note.UpdatedAt = pgtype.Timestamp{Time: time.Now(), Valid: true}

	tx, err := nr.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return &note, apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	query := `
		update public.notes
		set
			updated_at = $1
		where 1=1
			and notes.id = $2
			and notes.user_id = $3
			and notes.deleted_at is null;
	`
	tag, err := tx.Exec(ctx, query, note.UpdatedAt, note.Id, userId)
	if err != nil {
		return &note, apperrors.NewRepositoryError(err)
	}

	if tag.RowsAffected() == 0 {
		return &note, apperrors.ErrNotFound
	}

	query = `
		update public.notes_content
		set
			title = $1
		,	content = $2
		,	color = $3
		from
			public.notes
		where notes_content.note_id = notes.id
			and notes_content.note_id = $4
			and notes.user_id = $5
			and notes.deleted_at is null;
	`
	tag, err = tx.Exec(ctx, query, note.Title, note.Content, note.Color, note.Id, userId)
	if err != nil {
		return &note, apperrors.NewRepositoryError(err)
	}

	if tag.RowsAffected() == 0 {
		return &note, apperrors.ErrNotFound
	}

	if err := nr.syncNoteTags(ctx, tx, userId, int64(id), tagIDs); err != nil {
		return &note, err
	}

	if err := tx.Commit(ctx); err != nil {
		return &note, apperrors.NewRepositoryError(err)
	}

	return &note, nil
}

func (nr *NoteRepository) Delete(ctx context.Context, userId, id int) error {
	return nr.MoveToTrash(ctx, userId, id)
}

// -----------------------------------------------------------------------------
// Listagens
// -----------------------------------------------------------------------------

func (nr *NoteRepository) List(ctx context.Context, userId int, filter models.NoteFilter) ([]models.Note, error) {
	args := []any{userId}
	conditions := `
			and notes.user_id = $1
			and notes.archived_at is null
			and notes.deleted_at is null
	`

	// searchParam guarda o índice do placeholder do padrão de busca (já escapado
	// para LIKE): a ordenação por relevância precisa referenciar o mesmo
	// parâmetro que o filtro usa.
	searchParam := 0
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Search != "" {
		args = append(args, escapeLikePattern(filter.Search))
		searchParam = len(args)
		conditions += noteSearchConditionSQL(searchParam)
	}

	filter.Color = validations.NormalizeHexColor(filter.Color)
	if filter.Color != "" {
		args = append(args, filter.Color)
		conditions += fmt.Sprintf(`
			and notes_content.color = $%d
		`, len(args))
	}

	filter.Tag = strings.TrimSpace(filter.Tag)
	if filter.Tag != "" {
		args = append(args, filter.Tag)
		conditions += fmt.Sprintf(`
			and exists (
				select 1
				from public.note_tag_links ntl
				join public.note_tags nt on nt.id = ntl.tag_id
				where ntl.note_id = notes.id
					and nt.user_id = notes.user_id
					and nt.slug = $%d
			)
		`, len(args))
	}

	if filter.Pinned {
		conditions += `
			and notes.pinned = true
		`
	}

	// A relevância compara o título com o termo cru: o padrão escapado serve ao
	// LIKE, mas as barras de escape distorceriam a similaridade trigrama. O
	// parâmetro extra só é acrescentado quando a cláusula de fato o usa —
	// parâmetro sobrando faz o Postgres recusar o bind.
	orderBy := noteOrderBySQL(filter.Sort)
	if filter.Sort == models.NoteSortRelevance && searchParam > 0 {
		args = append(args, filter.Search)
		orderBy = noteRelevanceOrderBySQL(searchParam, len(args))
	}

	query := `
		select
			notes.id
		,	notes_content.title
		,	notes_content.content
		,	notes_content.color
		,	notes.pinned
		,	notes.archived_at
		,	notes.deleted_at
		,	notes.created_at
		,	notes.updated_at
		from
			public.notes
			inner join public.notes_content
				on notes.id = notes_content.note_id
		where 1=1
	` + conditions + `
		order by
	` + orderBy

	list, err := nr.queryNotes(ctx, query, args...)
	if err != nil {
		return list, err
	}

	return nr.loadTagsForNotes(ctx, list)
}

// queryNotes executa uma query de listagem de notas e faz o scan das colunas
// comuns a List, ListArchived e ListDeleted.
func (nr *NoteRepository) queryNotes(ctx context.Context, query string, args ...any) ([]models.Note, error) {
	var list []models.Note

	rows, err := nr.db.Query(ctx, query, args...)
	if err != nil {
		return list, apperrors.NewRepositoryError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var note models.Note
		err = rows.Scan(&note.Id, &note.Title, &note.Content, &note.Color, &note.Pinned, &note.ArchivedAt, &note.DeletedAt, &note.CreatedAt, &note.UpdatedAt)
		if err != nil {
			return list, apperrors.NewRepositoryError(err)
		}
		list = append(list, note)
	}

	if err := rows.Err(); err != nil {
		return list, apperrors.NewRepositoryError(err)
	}

	return list, nil
}

// escapeLikePattern neutraliza os metacaracteres de LIKE no termo buscado, para
// que "100%" ou "nota_1" sejam procurados literalmente em vez de virarem
// curinga. A barra invertida é o caractere de escape declarado no ESCAPE das
// comparações em noteSearchConditionSQL.
func escapeLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func noteSearchConditionSQL(param int) string {
	return fmt.Sprintf(`
		and (
			public.qn_unaccent(notes_content.title) ilike '%%' || public.qn_unaccent($%d) || '%%' escape '\'
			or public.qn_unaccent(notes_content.content) ilike '%%' || public.qn_unaccent($%d) || '%%' escape '\'
			or exists (
				select 1
				from
					public.note_tag_links
					inner join public.note_tags
						on note_tags.id = note_tag_links.tag_id
				where 1=1
					and note_tag_links.note_id = notes.id
					and note_tags.user_id = notes.user_id
					and public.qn_unaccent(note_tags.name) ilike '%%' || public.qn_unaccent($%d) || '%%' escape '\'
			)
		)
	`, param, param, param)
}

// noteOrderBySQL monta o ORDER BY da listagem sem busca. A relevância é tratada
// à parte, em noteRelevanceOrderBySQL, porque depende do termo buscado.
func noteOrderBySQL(sort string) string {
	switch sort {
	case models.NoteSortRecent:
		return `
			notes.updated_at desc
		,	notes.id desc;
		`
	case models.NoteSortOldest:
		return `
			notes.updated_at asc
		,	notes.id asc;
		`
	}

	return `
		notes.pinned desc
	,	notes.updated_at desc
	,	notes.id desc;
	`
}

// noteRelevanceOrderBySQL ordena os resultados de uma busca por relevância:
// nota com o termo no título vem antes da que só casa no conteúdo ou na
// etiqueta e, dentro de cada grupo, ordena pela proximidade trigrama entre o
// título e o termo (pg_trgm), caindo em recência no empate.
//
// patternParam é o padrão escapado, usado no LIKE; termParam é o termo cru,
// usado na similaridade — as barras de escape distorceriam o cálculo.
func noteRelevanceOrderBySQL(patternParam, termParam int) string {
	return fmt.Sprintf(`
		case
			when public.qn_unaccent(notes_content.title) ilike '%%' || public.qn_unaccent($%d) || '%%' escape '\' then 0
			else 1
		end
	,	similarity(public.qn_unaccent(notes_content.title), public.qn_unaccent($%d)) desc
	,	notes.updated_at desc
	,	notes.id desc;
	`, patternParam, termParam)
}

func (nr *NoteRepository) ListColors(ctx context.Context, userId int) ([]string, error) {
	var list []string

	query := `
		select distinct
			notes_content.color
		from
			public.notes_content
			inner join public.notes
				on notes_content.note_id = notes.id
		where 1=1
			and notes.user_id = $1
			and notes.archived_at is null
			and notes.deleted_at is null
		order by
			notes_content.color;
	`
	rows, err := nr.db.Query(ctx, query, userId)
	if err != nil {
		return list, apperrors.NewRepositoryError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var color string
		if err := rows.Scan(&color); err != nil {
			return list, apperrors.NewRepositoryError(err)
		}
		list = append(list, color)
	}

	if err := rows.Err(); err != nil {
		return list, apperrors.NewRepositoryError(err)
	}

	return list, nil
}

func (nr *NoteRepository) ListArchived(ctx context.Context, userId int) ([]models.Note, error) {
	query := `
		select
			notes.id
		,	notes_content.title
		,	notes_content.content
		,	notes_content.color
		,	notes.pinned
		,	notes.archived_at
		,	notes.deleted_at
		,	notes.created_at
		,	notes.updated_at
		from
			public.notes
			inner join public.notes_content
				on notes.id = notes_content.note_id
		where 1=1
			and notes.user_id = $1
			and notes.archived_at is not null
			and notes.deleted_at is null
		order by
			notes.archived_at desc
		,	notes.id desc;
	`

	list, err := nr.queryNotes(ctx, query, userId)
	if err != nil {
		return list, err
	}

	return nr.loadTagsForNotes(ctx, list)
}

func (nr *NoteRepository) ListDeleted(ctx context.Context, userId int) ([]models.Note, error) {
	query := `
		select
			notes.id
		,	notes_content.title
		,	notes_content.content
		,	notes_content.color
		,	notes.pinned
		,	notes.archived_at
		,	notes.deleted_at
		,	notes.created_at
		,	notes.updated_at
		from
			public.notes
			inner join public.notes_content
				on notes.id = notes_content.note_id
		where 1=1
			and notes.user_id = $1
			and notes.deleted_at is not null
		order by
			notes.deleted_at desc
		,	notes.id desc;
	`

	list, err := nr.queryNotes(ctx, query, userId)
	if err != nil {
		return list, err
	}

	return nr.loadTagsForNotes(ctx, list)
}

func (nr *NoteRepository) loadTagsForNotes(ctx context.Context, notes []models.Note) ([]models.Note, error) {
	if len(notes) == 0 {
		return notes, nil
	}

	noteIDs := make([]int64, 0, len(notes))
	for _, note := range notes {
		if note.Id.Int == nil {
			continue
		}
		noteIDs = append(noteIDs, note.Id.Int.Int64())
	}
	if len(noteIDs) == 0 {
		return notes, nil
	}

	query := `
		select
			note_tag_links.note_id
		,	note_tags.id
		,	note_tags.name
		,	note_tags.slug
		,	note_tags.color
		from
			public.note_tag_links
			inner join public.note_tags
				on note_tags.id = note_tag_links.tag_id
		where 1=1
			and note_tag_links.note_id = any($1::bigint[])
		order by
			note_tag_links.note_id
		,	note_tags.name;
	`
	rows, err := nr.db.Query(ctx, query, noteIDs)
	if err != nil {
		return notes, apperrors.NewRepositoryError(err)
	}
	defer rows.Close()

	tagsByNoteID := make(map[int64][]models.NoteTag, len(noteIDs))
	for rows.Next() {
		var noteID int64
		var tag models.NoteTag
		if err := rows.Scan(&noteID, &tag.Id, &tag.Name, &tag.Slug, &tag.Color); err != nil {
			return notes, apperrors.NewRepositoryError(err)
		}
		tagsByNoteID[noteID] = append(tagsByNoteID[noteID], tag)
	}

	if err := rows.Err(); err != nil {
		return notes, apperrors.NewRepositoryError(err)
	}

	for i := range notes {
		if notes[i].Id.Int == nil {
			continue
		}
		notes[i].Tags = tagsByNoteID[notes[i].Id.Int.Int64()]
	}

	return notes, nil
}

// -----------------------------------------------------------------------------
// Etiquetas
// -----------------------------------------------------------------------------

func (nr *NoteRepository) ListTags(ctx context.Context, userId int) ([]models.NoteTag, error) {
	var list []models.NoteTag

	query := `
		select
			note_tags.id
		,	note_tags.name
		,	note_tags.slug
		,	note_tags.color
		from
			public.note_tags
		where 1=1
			and note_tags.user_id = $1
		order by
			note_tags.name;
	`
	rows, err := nr.db.Query(ctx, query, userId)
	if err != nil {
		return list, apperrors.NewRepositoryError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var tag models.NoteTag
		if err := rows.Scan(&tag.Id, &tag.Name, &tag.Slug, &tag.Color); err != nil {
			return list, apperrors.NewRepositoryError(err)
		}
		list = append(list, tag)
	}

	if err := rows.Err(); err != nil {
		return list, apperrors.NewRepositoryError(err)
	}

	return list, nil
}

func (nr *NoteRepository) CreateTag(ctx context.Context, userId int, name, color string) (*models.NoteTag, error) {
	tagName := validations.NormalizeTagName(name)
	slug := noteTagSlug(tagName)
	if tagName == "" || slug == "" {
		return nil, apperrors.ErrNotFound
	}

	var tag models.NoteTag
	query := `
		insert into public.note_tags (user_id, name, slug, color)
		values ($1, $2, $3, $4)
		returning id, name, slug, color;
	`
	row := nr.db.QueryRow(ctx, query, userId, tagName, slug, nullableText(validations.NormalizeHexColor(color)))
	if err := row.Scan(&tag.Id, &tag.Name, &tag.Slug, &tag.Color); err != nil {
		if isUniqueViolation(err) {
			return nil, apperrors.ErrDuplicate
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	return &tag, nil
}

func (nr *NoteRepository) UpdateTag(ctx context.Context, userId, id int, name, color string) (*models.NoteTag, error) {
	tagName := validations.NormalizeTagName(name)
	slug := noteTagSlug(tagName)
	if tagName == "" || slug == "" {
		return nil, apperrors.ErrNotFound
	}

	var tag models.NoteTag
	query := `
		update public.note_tags
		set
			name = $1
		,	slug = $2
		,	color = $3
		,	updated_at = current_timestamp
		where 1=1
			and id = $4
			and user_id = $5
		returning id, name, slug, color;
	`
	row := nr.db.QueryRow(ctx, query, tagName, slug, nullableText(validations.NormalizeHexColor(color)), id, userId)
	if err := row.Scan(&tag.Id, &tag.Name, &tag.Slug, &tag.Color); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		if isUniqueViolation(err) {
			return nil, apperrors.ErrDuplicate
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	return &tag, nil
}

func (nr *NoteRepository) DeleteTag(ctx context.Context, userId, id int) error {
	query := `
		delete from public.note_tags
		where 1=1
			and id = $1
			and user_id = $2;
	`
	tag, err := nr.db.Exec(ctx, query, id, userId)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (nr *NoteRepository) syncNoteTags(ctx context.Context, tx pgx.Tx, userID int, noteID int64, tagIDs []int) error {
	ids := make([]int64, 0, len(tagIDs))
	for _, id := range validations.NormalizeTagIDs(tagIDs) {
		ids = append(ids, int64(id))
	}

	if len(ids) == 0 {
		query := `
			delete from public.note_tag_links
			using public.notes
			where 1=1
				and note_tag_links.note_id = notes.id
				and notes.id = $1
				and notes.user_id = $2;
		`
		if _, err := tx.Exec(ctx, query, noteID, userID); err != nil {
			return apperrors.NewRepositoryError(err)
		}
		return nil
	}

	query := `
		delete from public.note_tag_links
		using public.notes
		where 1=1
			and note_tag_links.note_id = notes.id
			and notes.id = $1
			and notes.user_id = $2
			and note_tag_links.tag_id <> all($3::bigint[]);
	`
	if _, err := tx.Exec(ctx, query, noteID, userID, ids); err != nil {
		return apperrors.NewRepositoryError(err)
	}

	query = `
		insert into public.note_tag_links (note_id, tag_id)
		select
			notes.id
		,	note_tags.id
		from
			public.notes
		join public.note_tags
			on note_tags.user_id = notes.user_id
		where 1=1
			and notes.id = $1
			and notes.user_id = $2
			and note_tags.id = any($3::bigint[])
		on conflict (note_id, tag_id) do nothing;
	`
	if _, err := tx.Exec(ctx, query, noteID, userID, ids); err != nil {
		return apperrors.NewRepositoryError(err)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Estado da nota (fixar, arquivar, lixeira)
// -----------------------------------------------------------------------------

func (nr *NoteRepository) execNoteState(ctx context.Context, query string, args ...any) error {
	tag, err := nr.db.Exec(ctx, query, args...)
	if err != nil {
		return apperrors.NewRepositoryError(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (nr *NoteRepository) SetPinned(ctx context.Context, userId int, id int, pinned bool) error {
	query := `
		update public.notes
		set
			pinned = $1
		,	updated_at = current_timestamp
		where 1=1
			and notes.id = $2
			and notes.user_id = $3
			and notes.archived_at is null
			and notes.deleted_at is null;
	`
	return nr.execNoteState(ctx, query, pinned, id, userId)
}

func (nr *NoteRepository) Archive(ctx context.Context, userId int, id int) error {
	query := `
		update public.notes
		set
			archived_at = current_timestamp
		,	pinned = false
		,	updated_at = current_timestamp
		where 1=1
			and notes.id = $1
			and notes.user_id = $2
			and notes.deleted_at is null
			and notes.archived_at is null;
	`
	return nr.execNoteState(ctx, query, id, userId)
}

func (nr *NoteRepository) Unarchive(ctx context.Context, userId int, id int) error {
	query := `
		update public.notes
		set
			archived_at = null
		,	updated_at = current_timestamp
		where 1=1
			and notes.id = $1
			and notes.user_id = $2
			and notes.deleted_at is null
			and notes.archived_at is not null;
	`
	return nr.execNoteState(ctx, query, id, userId)
}

func (nr *NoteRepository) MoveToTrash(ctx context.Context, userId int, id int) error {
	query := `
		update public.notes
		set
			deleted_at = current_timestamp
		,	pinned = false
		,	updated_at = current_timestamp
		where 1=1
			and notes.id = $1
			and notes.user_id = $2
			and notes.deleted_at is null;
	`
	return nr.execNoteState(ctx, query, id, userId)
}

func (nr *NoteRepository) Restore(ctx context.Context, userId int, id int) error {
	query := `
		update public.notes
		set
			deleted_at = null
		,	archived_at = null
		,	updated_at = current_timestamp
		where 1=1
			and notes.id = $1
			and notes.user_id = $2
			and notes.deleted_at is not null;
	`
	return nr.execNoteState(ctx, query, id, userId)
}

func (nr *NoteRepository) DeletePermanently(ctx context.Context, userId int, id int) error {
	query := `
		delete from public.notes
		where 1=1
			and notes.id = $1
			and notes.user_id = $2
			and notes.deleted_at is not null;
	`
	return nr.execNoteState(ctx, query, id, userId)
}

// -----------------------------------------------------------------------------
// Anexos
// -----------------------------------------------------------------------------

func (nr *NoteRepository) ListAttachments(ctx context.Context, userId, noteId int) ([]models.NoteAttachment, error) {
	var list []models.NoteAttachment

	query := `
		select
			note_attachments.id
		,	note_attachments.note_id
		,	note_attachments.user_id
		,	note_attachments.original_name
		,	note_attachments.storage_key
		,	note_attachments.mime_type
		,	note_attachments.size_bytes
		,	note_attachments.checksum_sha256
		,	note_attachments.created_at
		from
			public.note_attachments
		where 1=1
			and note_attachments.user_id = $1
			and note_attachments.note_id = $2
		order by
			note_attachments.created_at desc
		,	note_attachments.id desc;
	`
	rows, err := nr.db.Query(ctx, query, userId, noteId)
	if err != nil {
		return list, apperrors.NewRepositoryError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var attachment models.NoteAttachment
		err := rows.Scan(&attachment.Id, &attachment.NoteId, &attachment.UserId, &attachment.OriginalName, &attachment.StorageKey, &attachment.MimeType, &attachment.SizeBytes, &attachment.ChecksumSHA256, &attachment.CreatedAt)
		if err != nil {
			return list, apperrors.NewRepositoryError(err)
		}
		list = append(list, attachment)
	}

	if err := rows.Err(); err != nil {
		return list, apperrors.NewRepositoryError(err)
	}
	return list, nil
}

func (nr *NoteRepository) GetAttachment(ctx context.Context, userId, noteId, attachmentId int) (*models.NoteAttachment, error) {
	var attachment models.NoteAttachment
	query := `
		select
			note_attachments.id
		,	note_attachments.note_id
		,	note_attachments.user_id
		,	note_attachments.original_name
		,	note_attachments.storage_key
		,	note_attachments.mime_type
		,	note_attachments.size_bytes
		,	note_attachments.checksum_sha256
		,	note_attachments.created_at
		from
			public.note_attachments
		where 1=1
			and note_attachments.id = $1
			and note_attachments.note_id = $2
			and note_attachments.user_id = $3;
	`
	row := nr.db.QueryRow(ctx, query, attachmentId, noteId, userId)
	err := row.Scan(&attachment.Id, &attachment.NoteId, &attachment.UserId, &attachment.OriginalName, &attachment.StorageKey, &attachment.MimeType, &attachment.SizeBytes, &attachment.ChecksumSHA256, &attachment.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	return &attachment, nil
}

func (nr *NoteRepository) CreateAttachment(ctx context.Context, userId, noteId int, input NoteAttachmentInput) (*models.NoteAttachment, error) {
	var attachment models.NoteAttachment
	tx, err := nr.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	defer tx.Rollback(ctx)

	var lockedNoteID int64
	query := `
		select
			notes.id
		from
			public.notes
		where 1=1
			and notes.id = $1
			and notes.user_id = $2
			and notes.deleted_at is null
		for update;
	`
	if err := tx.QueryRow(ctx, query, noteId, userId).Scan(&lockedNoteID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.NewRepositoryError(err)
	}

	var count int
	query = `
		select count(*)
		from public.note_attachments
		where note_id = $1
			and user_id = $2;
	`
	if err := tx.QueryRow(ctx, query, noteId, userId).Scan(&count); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	if count >= validations.MaxNoteAttachmentsPerNote {
		return nil, apperrors.ErrAttachmentLimit
	}

	query = `
		insert into public.note_attachments (
			note_id
		,	user_id
		,	original_name
		,	storage_key
		,	mime_type
		,	size_bytes
		,	checksum_sha256
		) values ($1, $2, $3, $4, $5, $6, $7)
		returning
			id
		,	note_id
		,	user_id
		,	original_name
		,	storage_key
		,	mime_type
		,	size_bytes
		,	checksum_sha256
		,	created_at;
	`
	row := tx.QueryRow(ctx, query, noteId, userId, input.OriginalName, input.StorageKey, input.MimeType, input.SizeBytes, nullableText(input.ChecksumSHA256))
	err = row.Scan(&attachment.Id, &attachment.NoteId, &attachment.UserId, &attachment.OriginalName, &attachment.StorageKey, &attachment.MimeType, &attachment.SizeBytes, &attachment.ChecksumSHA256, &attachment.CreatedAt)
	if err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperrors.NewRepositoryError(err)
	}
	return &attachment, nil
}

func (nr *NoteRepository) DeleteAttachment(ctx context.Context, userId, noteId, attachmentId int) (*models.NoteAttachment, error) {
	var attachment models.NoteAttachment
	query := `
		delete from public.note_attachments
		using public.notes
		where 1=1
			and note_attachments.note_id = notes.id
			and note_attachments.id = $1
			and note_attachments.note_id = $2
			and note_attachments.user_id = $3
			and notes.user_id = $3
			and notes.deleted_at is null
		returning
			note_attachments.id
		,	note_attachments.note_id
		,	note_attachments.user_id
		,	note_attachments.original_name
		,	note_attachments.storage_key
		,	note_attachments.mime_type
		,	note_attachments.size_bytes
		,	note_attachments.checksum_sha256
		,	note_attachments.created_at;
	`
	row := nr.db.QueryRow(ctx, query, attachmentId, noteId, userId)
	err := row.Scan(&attachment.Id, &attachment.NoteId, &attachment.UserId, &attachment.OriginalName, &attachment.StorageKey, &attachment.MimeType, &attachment.SizeBytes, &attachment.ChecksumSHA256, &attachment.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.NewRepositoryError(err)
	}
	return &attachment, nil
}

func (nr *NoteRepository) loadAttachmentsForNotes(ctx context.Context, notes []models.Note) ([]models.Note, error) {
	if len(notes) == 0 {
		return notes, nil
	}
	noteIDs := make([]int64, 0, len(notes))
	for _, note := range notes {
		if note.Id.Int == nil {
			continue
		}
		noteIDs = append(noteIDs, note.Id.Int.Int64())
	}
	if len(noteIDs) == 0 {
		return notes, nil
	}

	query := `
		select
			note_attachments.id
		,	note_attachments.note_id
		,	note_attachments.user_id
		,	note_attachments.original_name
		,	note_attachments.storage_key
		,	note_attachments.mime_type
		,	note_attachments.size_bytes
		,	note_attachments.checksum_sha256
		,	note_attachments.created_at
		from
			public.note_attachments
		where 1=1
			and note_attachments.note_id = any($1::bigint[])
		order by
			note_attachments.note_id
		,	note_attachments.created_at desc
		,	note_attachments.id desc;
	`
	rows, err := nr.db.Query(ctx, query, noteIDs)
	if err != nil {
		return notes, apperrors.NewRepositoryError(err)
	}
	defer rows.Close()

	attachmentsByNoteID := make(map[int64][]models.NoteAttachment, len(noteIDs))
	for rows.Next() {
		var attachment models.NoteAttachment
		err := rows.Scan(&attachment.Id, &attachment.NoteId, &attachment.UserId, &attachment.OriginalName, &attachment.StorageKey, &attachment.MimeType, &attachment.SizeBytes, &attachment.ChecksumSHA256, &attachment.CreatedAt)
		if err != nil {
			return notes, apperrors.NewRepositoryError(err)
		}
		if attachment.NoteId.Int != nil {
			attachmentsByNoteID[attachment.NoteId.Int.Int64()] = append(attachmentsByNoteID[attachment.NoteId.Int.Int64()], attachment)
		}
	}
	if err := rows.Err(); err != nil {
		return notes, apperrors.NewRepositoryError(err)
	}

	for i := range notes {
		if notes[i].Id.Int == nil {
			continue
		}
		notes[i].Attachments = attachmentsByNoteID[notes[i].Id.Int.Int64()]
	}
	return notes, nil
}

// -----------------------------------------------------------------------------
// Normalização e helpers
// -----------------------------------------------------------------------------

func noteTagSlug(name string) string {
	return validations.NormalizeTagSlug(name)
}

func nullableText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{String: value, Valid: value != ""}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
