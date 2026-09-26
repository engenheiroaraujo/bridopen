package handlers

import (
	"bytes"
	"context"
	"errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
	"io"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	notemodel "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	noterepo "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/repositories"
	noteservice "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/service"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
	"github.com/gorilla/csrf"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func numID(id int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(id), Valid: true}
}

func textVal(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

type fakeNoteRepo struct {
	notes           []notemodel.Note
	colors          []string
	tags            []notemodel.NoteTag
	note            *notemodel.Note
	created         *notemodel.Note
	updated         *notemodel.Note
	tag             *notemodel.NoteTag
	attachment      *notemodel.NoteAttachment
	attachments     []notemodel.NoteAttachment
	listErr         error
	colorsErr       error
	tagsErr         error
	getErr          error
	createErr       error
	updateErr       error
	deleteErr       error
	archiveListErr  error
	trashListErr    error
	setPinnedErr    error
	archiveErr      error
	unarchiveErr    error
	restoreErr      error
	deletePermErr   error
	listAttachErr   error
	getAttachErr    error
	createAttachErr error
	deleteAttachErr error
	createTagErr    error
	updateTagErr    error
	deleteTagErr    error
	deletedAttach   *notemodel.NoteAttachment
	getCalls        int
	getErrAfter     int // se >0, devolve getErr a partir desta chamada (1-based)
}

func (f *fakeNoteRepo) List(context.Context, int, notemodel.NoteFilter) ([]notemodel.Note, error) {
	return f.notes, f.listErr
}
func (f *fakeNoteRepo) ListColors(context.Context, int) ([]string, error) {
	return f.colors, f.colorsErr
}
func (f *fakeNoteRepo) ListTags(context.Context, int) ([]notemodel.NoteTag, error) {
	return f.tags, f.tagsErr
}
func (f *fakeNoteRepo) CreateTag(context.Context, int, string, string) (*notemodel.NoteTag, error) {
	if f.createTagErr != nil {
		return nil, f.createTagErr
	}
	if f.tag == nil {
		t := notemodel.NoteTag{Id: numID(3), Name: textVal("tag"), Color: textVal("#2f4538")}
		return &t, nil
	}
	return f.tag, nil
}
func (f *fakeNoteRepo) UpdateTag(context.Context, int, int, string, string) (*notemodel.NoteTag, error) {
	if f.updateTagErr != nil {
		return nil, f.updateTagErr
	}
	if f.tag == nil {
		t := notemodel.NoteTag{Id: numID(3), Name: textVal("tag"), Color: textVal("#2f4538")}
		return &t, nil
	}
	return f.tag, nil
}
func (f *fakeNoteRepo) DeleteTag(context.Context, int, int) error { return f.deleteTagErr }
func (f *fakeNoteRepo) GetById(context.Context, int, int) (*notemodel.Note, error) {
	f.getCalls++
	if f.getErr != nil && (f.getErrAfter == 0 || f.getCalls >= f.getErrAfter) {
		return nil, f.getErr
	}
	if f.note == nil {
		n := sampleNote(1)
		return &n, nil
	}
	return f.note, nil
}
func (f *fakeNoteRepo) Create(context.Context, int, string, string, string, []int) (*notemodel.Note, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.created != nil {
		return f.created, nil
	}
	n := sampleNote(9)
	return &n, nil
}
func (f *fakeNoteRepo) Update(context.Context, int, int, string, string, string, []int) (*notemodel.Note, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	if f.updated != nil {
		return f.updated, nil
	}
	n := sampleNote(1)
	return &n, nil
}
func (f *fakeNoteRepo) Delete(context.Context, int, int) error { return f.deleteErr }
func (f *fakeNoteRepo) ListArchived(context.Context, int) ([]notemodel.Note, error) {
	return f.notes, f.archiveListErr
}
func (f *fakeNoteRepo) ListDeleted(context.Context, int) ([]notemodel.Note, error) {
	return f.notes, f.trashListErr
}
func (f *fakeNoteRepo) SetPinned(context.Context, int, int, bool) error { return f.setPinnedErr }
func (f *fakeNoteRepo) Archive(context.Context, int, int) error         { return f.archiveErr }
func (f *fakeNoteRepo) Unarchive(context.Context, int, int) error       { return f.unarchiveErr }
func (f *fakeNoteRepo) MoveToTrash(context.Context, int, int) error     { return nil }
func (f *fakeNoteRepo) Restore(context.Context, int, int) error         { return f.restoreErr }
func (f *fakeNoteRepo) DeletePermanently(context.Context, int, int) error {
	return f.deletePermErr
}
func (f *fakeNoteRepo) ListAttachments(context.Context, int, int) ([]notemodel.NoteAttachment, error) {
	return f.attachments, f.listAttachErr
}
func (f *fakeNoteRepo) GetAttachment(context.Context, int, int, int) (*notemodel.NoteAttachment, error) {
	if f.getAttachErr != nil {
		return nil, f.getAttachErr
	}
	if f.attachment == nil {
		a := sampleAttachment(5)
		return &a, nil
	}
	return f.attachment, nil
}
func (f *fakeNoteRepo) CreateAttachment(context.Context, int, int, noterepo.NoteAttachmentInput) (*notemodel.NoteAttachment, error) {
	if f.createAttachErr != nil {
		return nil, f.createAttachErr
	}
	if f.attachment == nil {
		a := sampleAttachment(5)
		return &a, nil
	}
	return f.attachment, nil
}
func (f *fakeNoteRepo) DeleteAttachment(context.Context, int, int, int) (*notemodel.NoteAttachment, error) {
	if f.deleteAttachErr != nil {
		return nil, f.deleteAttachErr
	}
	if f.deletedAttach != nil {
		return f.deletedAttach, nil
	}
	a := sampleAttachment(5)
	return &a, nil
}

func sampleNote(id int64) notemodel.Note {
	return notemodel.Note{
		Id:      numID(id),
		Title:   textVal("Titulo"),
		Content: textVal("Conteudo"),
		Color:   textVal("#ffffff"),
	}
}

func sampleAttachment(id int64) notemodel.NoteAttachment {
	return notemodel.NoteAttachment{
		Id:           numID(id),
		OriginalName: textVal("file.png"),
		StorageKey:   textVal("1/1/file.png"),
		MimeType:     textVal("image/png"),
	}
}

type fakeAttachmentStorage struct {
	saveErr   error
	pathErr   error
	deleteErr error
	path      string
	stored    noteservice.StoredAttachment
	deleted   []string
}

func (f *fakeAttachmentStorage) Save(context.Context, int, int, multipart.File, *multipart.FileHeader) (noteservice.StoredAttachment, error) {
	if f.saveErr != nil {
		return noteservice.StoredAttachment{}, f.saveErr
	}
	if f.stored.StorageKey != "" {
		return f.stored, nil
	}
	return noteservice.StoredAttachment{
		OriginalName:   "file.png",
		StorageKey:     "1/1/file.png",
		MimeType:       "image/png",
		SizeBytes:      10,
		ChecksumSHA256: "abc",
	}, nil
}
func (f *fakeAttachmentStorage) Path(string) (string, error) {
	if f.pathErr != nil {
		return "", f.pathErr
	}
	if f.path != "" {
		return f.path, nil
	}
	return "", apperrors.ErrAttachmentInvalidPath
}
func (f *fakeAttachmentStorage) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return f.deleteErr
}

func newNoteHandlerForTest(t *testing.T, repo *fakeNoteRepo, storage *fakeAttachmentStorage) (*noteHandler, *scs.SessionManager) {
	t.Helper()
	session := scs.New()
	if repo == nil {
		repo = &fakeNoteRepo{}
	}
	if storage == nil {
		storage = &fakeAttachmentStorage{}
	}
	svc := noteservice.NewNoteService(repo, storage, audit.NewRecorder(nil))
	h := NewNoteHandler(render.NewRender(session, "http://example.com"), session, svc)
	return h, session
}

var testCSRFKey = []byte("01234567890123456789012345678901")

func csrfProtect(next http.Handler) http.Handler {
	return csrf.Protect(
		testCSRFKey,
		csrf.Secure(false),
		csrf.Path("/"),
		csrf.TrustedOrigins([]string{"example.com"}),
	)(next)
}

func serveNoteHandler(t *testing.T, session *scs.SessionManager, userID int64, fn func(http.ResponseWriter, *http.Request) error, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	req.Host = "example.com"
	req.URL.Scheme = "http"
	req.URL.Host = "example.com"
	req.Header.Set("Referer", "http://example.com/")
	req.Header.Del("Origin")

	rec := httptest.NewRecorder()
	var token string
	var cookies []*http.Cookie

	// Emite token+cookie no mesmo session manager e depois reaplica no request real.
	issue := csrfProtect(session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = csrf.Token(r)
		w.WriteHeader(http.StatusNoContent)
	})))
	issueRec := httptest.NewRecorder()
	issueReq := csrf.PlaintextHTTPRequest(httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	issueReq.Host = "example.com"
	issue.ServeHTTP(issueRec, issueReq)
	cookies = issueRec.Result().Cookies()

	req.Header.Set("X-CSRF-Token", token)
	for _, c := range cookies {
		req.AddCookie(c)
	}

	handler := csrfProtect(session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID > 0 {
			session.Put(r.Context(), "userId", userID)
		}
		err := fn(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})))
	handler.ServeHTTP(rec, csrf.PlaintextHTTPRequest(req))
	return rec
}

func formRequest(method, target string, values url.Values) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func multipartRequest(t *testing.T, target, field, filename, content string) *http.Request {
	t.Helper()
	return multipartRequestRedirect(t, target, field, filename, content, "/note/1/edit")
}

func multipartRequestRedirect(t *testing.T, target, field, filename, content, redirect string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(field, filename)
	require.NoError(t, err)
	_, err = io.WriteString(part, content)
	require.NoError(t, err)
	require.NoError(t, w.WriteField("redirect", redirect))
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, target, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.SetPathValue("id", "1")
	return req
}

// TestNoteListFilterFromRequestParsesSearchDirectives verifica o parse de diretivas de busca (#tag, cor:, fixadas) na query string.
func TestNoteListFilterFromRequestParsesSearchDirectives(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "/?q=reuniao%20%23Trabalho%20fixadas%20cor:%23AABBCC", nil)

	filter := noteListFilterFromRequest(req)

	assert.Equal(t, "reuniao", filter.Search)
	assert.Equal(t, "trabalho", filter.Tag)
	assert.Equal(t, "#aabbcc", filter.Color)
	assert.True(t, filter.Pinned)
	assert.Equal(t, notemodel.NoteSortRelevance, filter.Sort)
}

// TestNoteListFilterExplicitParamsWinOverSearchDirectives verifica que parâmetros explícitos prevalecem sobre as diretivas da busca.
func TestNoteListFilterExplicitParamsWinOverSearchDirectives(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "/?q=%23Trabalho%20cor:%23AABBCC&tag=pessoal&color=%23ffffff&sort=recent", nil)

	filter := noteListFilterFromRequest(req)

	assert.Equal(t, "pessoal", filter.Tag)
	assert.Equal(t, "#ffffff", filter.Color)
	assert.Equal(t, notemodel.NoteSortRecent, filter.Sort)
}

// TestNoteListFilterDefaultsAndInvalidDirectives verifica ordenações padrão, valores inválidos e diretivas malformadas.
func TestNoteListFilterDefaultsAndInvalidDirectives(t *testing.T) {
	t.Parallel()

	filter := noteListFilterFromRequest(httptest.NewRequest("GET", "/", nil))
	assert.Equal(t, notemodel.NoteSortPinned, filter.Sort)
	assert.Empty(t, filter.Search)

	filter = noteListFilterFromRequest(httptest.NewRequest("GET", "/?q=hello&sort=relevance", nil))
	assert.Equal(t, notemodel.NoteSortRelevance, filter.Sort)

	filter = noteListFilterFromRequest(httptest.NewRequest("GET", "/?sort=oldest", nil))
	assert.Equal(t, notemodel.NoteSortOldest, filter.Sort)

	filter = noteListFilterFromRequest(httptest.NewRequest("GET", "/?q=cor:%23zz%20pinned&pinned=true", nil))
	assert.True(t, filter.Pinned)
	assert.Contains(t, filter.Search, "cor:#zz")

	search, parsed := parseNoteSearchDirectives("   ")
	assert.Empty(t, search)
	assert.False(t, parsed.Pinned)

	search, parsed = parseNoteSearchDirectives("#")
	assert.Equal(t, "#", search)
	assert.Empty(t, parsed.Tag)

	search, parsed = parseNoteSearchDirectives("#@@@ hello")
	assert.Equal(t, "#@@@ hello", search)
	assert.Empty(t, parsed.Tag)
}

// TestNewNoteHandlerAndHelpers verifica a construção do handler e helpers de sessão, IDs, redirect, JSON e sanitização.
func TestNewNoteHandlerAndHelpers(t *testing.T) {
	t.Parallel()

	h, session := newNoteHandlerForTest(t, nil, nil)
	assert.NotNil(t, h)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx, err := session.Load(req.Context(), "s1")
	require.NoError(t, err)
	session.Put(ctx, "userId", int64(7))
	req = req.WithContext(ctx)
	assert.Equal(t, int64(7), h.getUserIdFromSession(req))

	req = httptest.NewRequest(http.MethodGet, "/note/x", nil)
	req.SetPathValue("id", "x")
	_, err = noteIDFromRequest(req)
	require.Error(t, err)
	req.SetPathValue("id", "3")
	id, err := noteIDFromRequest(req)
	require.NoError(t, err)
	assert.Equal(t, 3, id)

	req.SetPathValue("attachmentId", "bad")
	_, err = noteAttachmentIDFromRequest(req)
	require.Error(t, err)
	req.SetPathValue("attachmentId", "4")
	aid, err := noteAttachmentIDFromRequest(req)
	require.NoError(t, err)
	assert.Equal(t, 4, aid)

	assert.Equal(t, "/", noteActionRedirect(httptest.NewRequest(http.MethodPost, "/", nil), "/"))
	bad := formRequest(http.MethodPost, "/", url.Values{"redirect": {"//evil.com"}})
	require.NoError(t, bad.ParseForm())
	assert.Equal(t, "/ok", noteActionRedirect(bad, "/ok"))
	good := formRequest(http.MethodPost, "/", url.Values{"redirect": {"/notes/trash"}})
	require.NoError(t, good.ParseForm())
	assert.Equal(t, "/notes/trash", noteActionRedirect(good, "/ok"))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept", "application/json")
	assert.True(t, noteWantsJSON(r))
	assert.True(t, noteTagWantsJSON(r))
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("X-Requested-With", "XMLHttpRequest")
	assert.True(t, noteWantsJSON(r2))
	assert.False(t, noteWantsJSON(httptest.NewRequest(http.MethodGet, "/", nil)))

	assert.Equal(t, "", validations.TruncateRunes("abc", 0))
	assert.Equal(t, "ab", validations.TruncateRunes("abcd", 2))
	assert.Equal(t, "", sanitizeNoteColorFilter("red"))
	assert.Equal(t, "", sanitizeNoteColorFilter("#gg0000"))
	assert.Equal(t, "", sanitizeNoteColorFilter("#12345"))
}

// TestNoteListViewNewArchiveTrash verifica listagem, visualização, nova nota, arquivo e lixeira em sucesso e erro.
func TestNoteListViewNewArchiveTrash(t *testing.T) {
	t.Parallel()

	repo := &fakeNoteRepo{notes: []notemodel.Note{sampleNote(1)}, colors: []string{"#fff"}, tags: []notemodel.NoteTag{{Id: numID(1), Name: textVal("t")}}}
	h, session := newNoteHandlerForTest(t, repo, nil)

	rec := serveNoteHandler(t, session, 1, h.NoteList, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = serveNoteHandler(t, session, 1, h.NoteList, httptest.NewRequest(http.MethodGet, "/other", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	repo.listErr = errors.New("list boom")
	rec = serveNoteHandler(t, session, 1, h.NoteList, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.listErr = nil
	repo.colorsErr = errors.New("colors")
	rec = serveNoteHandler(t, session, 1, h.NoteList, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.colorsErr = nil
	repo.tagsErr = errors.New("tags")
	rec = serveNoteHandler(t, session, 1, h.NoteList, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.tagsErr = nil

	req := httptest.NewRequest(http.MethodGet, "/note/1", nil)
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteView, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/note/x", nil)
	req.SetPathValue("id", "x")
	rec = serveNoteHandler(t, session, 1, h.NoteView, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	repo.getErr = apperrors.ErrNotFound
	req = httptest.NewRequest(http.MethodGet, "/note/1", nil)
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteView, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getErr = errors.New("db")
	rec = serveNoteHandler(t, session, 1, h.NoteView, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getErr = nil

	rec = serveNoteHandler(t, session, 1, h.NoteNew, httptest.NewRequest(http.MethodGet, "/note/new", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	repo.tagsErr = errors.New("tags")
	rec = serveNoteHandler(t, session, 1, h.NoteNew, httptest.NewRequest(http.MethodGet, "/note/new", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.tagsErr = nil

	rec = serveNoteHandler(t, session, 1, h.NoteArchiveList, httptest.NewRequest(http.MethodGet, "/notes/archive", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	repo.archiveListErr = errors.New("arch")
	rec = serveNoteHandler(t, session, 1, h.NoteArchiveList, httptest.NewRequest(http.MethodGet, "/notes/archive", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.archiveListErr = nil

	rec = serveNoteHandler(t, session, 1, h.NoteTrashList, httptest.NewRequest(http.MethodGet, "/notes/trash", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	repo.trashListErr = errors.New("trash")
	rec = serveNoteHandler(t, session, 1, h.NoteTrashList, httptest.NewRequest(http.MethodGet, "/notes/trash", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestNoteSaveEditDelete verifica salvar, editar e excluir notas, incluindo validação e falhas.
func TestNoteSaveEditDelete(t *testing.T) {
	t.Parallel()

	repo := &fakeNoteRepo{}
	h, session := newNoteHandlerForTest(t, repo, nil)

	// validação na criação
	req := formRequest(http.MethodPost, "/note", url.Values{"title": {""}, "content": {""}})
	rec := serveNoteHandler(t, session, 1, h.NoteSave, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// validação na atualização
	longTitle := strings.Repeat("a", 60)
	req = formRequest(http.MethodPost, "/note", url.Values{"id": {"1"}, "title": {longTitle}, "content": {"x"}})
	rec = serveNoteHandler(t, session, 1, h.NoteSave, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// seleção de tag inválida
	req = formRequest(http.MethodPost, "/note", url.Values{"title": {"T"}, "content": {"C"}, "tag_ids": {"999"}})
	rec = serveNoteHandler(t, session, 1, h.NoteSave, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// criação com sucesso
	req = formRequest(http.MethodPost, "/note", url.Values{"title": {"T"}, "content": {"C"}, "color": {"#ffffff"}})
	rec = serveNoteHandler(t, session, 1, h.NoteSave, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	// atualização com sucesso
	req = formRequest(http.MethodPost, "/note", url.Values{"id": {"1"}, "title": {"T"}, "content": {"C"}})
	rec = serveNoteHandler(t, session, 1, h.NoteSave, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	// criação sem id.Int para o ramo de auditoria
	repo.created = &notemodel.Note{Title: textVal("T"), Content: textVal("C")}
	req = formRequest(http.MethodPost, "/note", url.Values{"title": {"T"}, "content": {"C"}})
	rec = serveNoteHandler(t, session, 1, h.NoteSave, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	repo.created = nil

	repo.tagsErr = errors.New("tags")
	req = formRequest(http.MethodPost, "/note", url.Values{"title": {"T"}, "content": {"C"}})
	rec = serveNoteHandler(t, session, 1, h.NoteSave, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.tagsErr = nil

	repo.createErr = errors.New("create")
	req = formRequest(http.MethodPost, "/note", url.Values{"title": {"T"}, "content": {"C"}})
	rec = serveNoteHandler(t, session, 1, h.NoteSave, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.createErr = nil

	// Erro de ParseForm via MaxBytesReader já consumido? Corpo inválido com content-length errado é difícil.
	// Usa um request que falha no ParseForm: Method POST com Body que retorna erro.
	badBody := &errReader{}
	req = httptest.NewRequest(http.MethodPost, "/note", badBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveNoteHandler(t, session, 1, h.NoteSave, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// edição
	req = httptest.NewRequest(http.MethodGet, "/note/1/edit", nil)
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteEdit, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	req.SetPathValue("id", "0")
	rec = serveNoteHandler(t, session, 1, h.NoteEdit, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	repo.getErr = apperrors.ErrNotFound
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteEdit, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getErr = errors.New("db")
	rec = serveNoteHandler(t, session, 1, h.NoteEdit, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getErr = nil
	repo.tagsErr = errors.New("tags")
	rec = serveNoteHandler(t, session, 1, h.NoteEdit, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.tagsErr = nil

	// exclusão com redirect + JSON
	rec = serveNoteHandler(t, session, 1, h.NoteDelete, notePathPost("/note/1/delete", "1"))
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	req = notePathPost("/note/1/delete", "1")
	req.Header.Set("Accept", "application/json")
	rec = serveNoteHandler(t, session, 1, h.NoteDelete, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":true`)

	rec = serveNoteHandler(t, session, 1, h.NoteDelete, notePathPost("/note/1/delete", "bad"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	repo.deleteErr = errors.New("del")
	rec = serveNoteHandler(t, session, 1, h.NoteDelete, notePathPost("/note/1/delete", "1"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read fail") }

func notePathPost(path, id string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.SetPathValue("id", id)
	return req
}

// TestNoteStateActions verifica ações de estado (fixar, arquivar, restaurar e exclusão permanente).
func TestNoteStateActions(t *testing.T) {
	t.Parallel()

	repo := &fakeNoteRepo{}
	storage := &fakeAttachmentStorage{deleteErr: errors.New("disk")}
	h, session := newNoteHandlerForTest(t, repo, storage)

	actions := []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request) error
		errp *error
	}{
		{"pin", h.NotePin, &repo.setPinnedErr},
		{"unpin", h.NoteUnpin, &repo.setPinnedErr},
		{"archive", h.NoteArchive, &repo.archiveErr},
		{"unarchive", h.NoteUnarchive, &repo.unarchiveErr},
		{"restore", h.NoteRestore, &repo.restoreErr},
	}
	for _, tc := range actions {
		t.Run(tc.name, func(t *testing.T) {
			*tc.errp = nil
			rec := serveNoteHandler(t, session, 1, tc.fn, notePathPost("/note/1/"+tc.name, "1"))
			assert.Equal(t, http.StatusSeeOther, rec.Code)

			rec = serveNoteHandler(t, session, 1, tc.fn, notePathPost("/note/1/"+tc.name, "x"))
			assert.Equal(t, http.StatusInternalServerError, rec.Code)

			*tc.errp = errors.New(tc.name + " boom")
			rec = serveNoteHandler(t, session, 1, tc.fn, notePathPost("/note/1/"+tc.name, "1"))
			assert.Equal(t, http.StatusInternalServerError, rec.Code)
			*tc.errp = nil
		})
	}

	repo.attachments = []notemodel.NoteAttachment{sampleAttachment(1), {StorageKey: pgtype.Text{Valid: false}}}
	rec := serveNoteHandler(t, session, 1, h.NoteDeletePermanently, notePathPost("/note/1/delete-permanent", "1"))
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	req := notePathPost("/note/1/delete-permanent", "1")
	req.Header.Set("Accept", "application/json")
	rec = serveNoteHandler(t, session, 1, h.NoteDeletePermanently, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	repo.listAttachErr = errors.New("list")
	rec = serveNoteHandler(t, session, 1, h.NoteDeletePermanently, notePathPost("/note/1/delete-permanent", "1"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.listAttachErr = nil
	repo.deletePermErr = errors.New("perm")
	rec = serveNoteHandler(t, session, 1, h.NoteDeletePermanently, notePathPost("/note/1/delete-permanent", "1"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = serveNoteHandler(t, session, 1, h.NoteDeletePermanently, notePathPost("/note/1/delete-permanent", "bad"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestNoteTags verifica o CRUD de etiquetas em HTML e JSON, incluindo duplicatas e erros.
func TestNoteTags(t *testing.T) {
	t.Parallel()

	repo := &fakeNoteRepo{tags: []notemodel.NoteTag{{Id: numID(1), Name: textVal("a"), Color: textVal("#2f4538")}}}
	h, session := newNoteHandlerForTest(t, repo, nil)

	rec := serveNoteHandler(t, session, 1, h.NoteTagList, httptest.NewRequest(http.MethodGet, "/tags", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	// criação inválida HTML
	req := formRequest(http.MethodPost, "/tags", url.Values{"name": {""}, "color": {"bad"}})
	rec = serveNoteHandler(t, session, 1, h.NoteTagCreate, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// criação inválida JSON
	req = formRequest(http.MethodPost, "/tags", url.Values{"name": {""}, "color": {"bad"}})
	req.Header.Set("Accept", "application/json")
	rec = serveNoteHandler(t, session, 1, h.NoteTagCreate, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// criação com sucesso + JSON
	req = formRequest(http.MethodPost, "/tags", url.Values{"name": {"Work"}, "color": {"#2f4538"}})
	rec = serveNoteHandler(t, session, 1, h.NoteTagCreate, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	req = formRequest(http.MethodPost, "/tags", url.Values{"name": {"Work"}, "color": {"#2f4538"}})
	req.Header.Set("Accept", "application/json")
	rec = serveNoteHandler(t, session, 1, h.NoteTagCreate, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	repo.createTagErr = apperrors.ErrDuplicate
	req = formRequest(http.MethodPost, "/tags", url.Values{"name": {"Work"}, "color": {"#2f4538"}})
	rec = serveNoteHandler(t, session, 1, h.NoteTagCreate, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	req = formRequest(http.MethodPost, "/tags", url.Values{"name": {"Work"}, "color": {"#2f4538"}})
	req.Header.Set("Accept", "application/json")
	rec = serveNoteHandler(t, session, 1, h.NoteTagCreate, req)
	assert.Equal(t, http.StatusConflict, rec.Code)
	repo.createTagErr = errors.New("db")
	rec = serveNoteHandler(t, session, 1, h.NoteTagCreate, formRequest(http.MethodPost, "/tags", url.Values{"name": {"Work"}, "color": {"#2f4538"}}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.createTagErr = nil

	req = httptest.NewRequest(http.MethodPost, "/tags", &errReader{})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = serveNoteHandler(t, session, 1, h.NoteTagCreate, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// atualização
	req = formRequest(http.MethodPost, "/tags/1", url.Values{"name": {""}, "color": {"#2f4538"}})
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagUpdate, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	req = formRequest(http.MethodPost, "/tags/1", url.Values{"name": {"Work"}, "color": {"#2f4538"}})
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagUpdate, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	repo.updateTagErr = apperrors.ErrNotFound
	req = formRequest(http.MethodPost, "/tags/1", url.Values{"name": {"Work"}, "color": {"#2f4538"}})
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagUpdate, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.updateTagErr = apperrors.ErrDuplicate
	req = formRequest(http.MethodPost, "/tags/1", url.Values{"name": {"Work"}, "color": {"#2f4538"}})
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagUpdate, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	repo.updateTagErr = errors.New("db")
	req = formRequest(http.MethodPost, "/tags/1", url.Values{"name": {"Work"}, "color": {"#2f4538"}})
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagUpdate, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.updateTagErr = nil

	req = formRequest(http.MethodPost, "/tags/1", url.Values{"name": {"Work"}, "color": {"#2f4538"}})
	req.SetPathValue("id", "bad")
	rec = serveNoteHandler(t, session, 1, h.NoteTagUpdate, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/tags/1", &errReader{})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagUpdate, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// exclusão
	req = formRequest(http.MethodPost, "/tags/1/delete", url.Values{})
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagDelete, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	repo.deleteTagErr = apperrors.ErrNotFound
	req = formRequest(http.MethodPost, "/tags/1/delete", url.Values{})
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagDelete, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.deleteTagErr = errors.New("db")
	req = formRequest(http.MethodPost, "/tags/1/delete", url.Values{})
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagDelete, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	req = formRequest(http.MethodPost, "/tags/1/delete", url.Values{})
	req.SetPathValue("id", "0")
	rec = serveNoteHandler(t, session, 1, h.NoteTagDelete, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/tags/1/delete", &errReader{})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteTagDelete, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	repo.tagsErr = errors.New("tags list")
	rec = serveNoteHandler(t, session, 1, h.NoteTagList, httptest.NewRequest(http.MethodGet, "/tags", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestNoteAttachments verifica upload, download e exclusão de anexos, além das mensagens de erro.
func TestNoteAttachments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.png")
	require.NoError(t, os.WriteFile(filePath, []byte("pngdata"), 0o600))

	repo := &fakeNoteRepo{note: ptrNote(sampleNote(1))}
	storage := &fakeAttachmentStorage{path: filePath}
	h, session := newNoteHandlerForTest(t, repo, storage)

	req := multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello")
	rec := serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	// caminho de erro de parse por tamanho excessivo
	req = httptest.NewRequest(http.MethodPost, "/note/1/attachments", strings.NewReader("x"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=abc")
	req.SetPathValue("id", "1")
	// MaxBytesReader dispara com corpo grande demais — simular via ContentLength enorme com corpo pequeno ainda pode falhar no parse
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, req)
	assert.True(t, rec.Code == http.StatusRequestEntityTooLarge || rec.Code == http.StatusUnprocessableEntity || rec.Code == http.StatusInternalServerError)

	req = multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello")
	req.SetPathValue("id", "bad")
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	repo.getErr = apperrors.ErrNotFound
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getErr = errors.New("db")
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getErr = nil

	deleted := sampleNote(1)
	deleted.DeletedAt = pgtype.Timestamp{Valid: true, Time: time.Now()}
	repo.note = &deleted
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	repo.note = ptrNote(sampleNote(1))

	// campo de arquivo ausente (multipart válido sem parte attachment)
	var noFile bytes.Buffer
	mw := multipart.NewWriter(&noFile)
	require.NoError(t, mw.WriteField("redirect", "/note/1/edit"))
	require.NoError(t, mw.Close())
	req = httptest.NewRequest(http.MethodPost, "/note/1/attachments", &noFile)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("id", "1")
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	storage.saveErr = apperrors.ErrAttachmentTooLarge
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	storage.saveErr = nil

	repo.createAttachErr = apperrors.ErrAttachmentLimit
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	repo.createAttachErr = apperrors.ErrNotFound
	storage.deleteErr = errors.New("cleanup")
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.createAttachErr = errors.New("db")
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.createAttachErr = nil
	storage.deleteErr = nil

	// erro de render do anexo na view (sem /edit) + falha de ListTags na edição
	storage.saveErr = apperrors.ErrAttachmentTypeNotAllowed
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequestRedirect(t, "/note/1/attachments", "attachment", "a.png", "hello", "/note/1"))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	storage.saveErr = nil

	repo.tagsErr = errors.New("tags")
	storage.saveErr = apperrors.ErrAttachmentEmpty
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.tagsErr = nil
	storage.saveErr = nil

	storage.saveErr = apperrors.ErrAttachmentInvalidName
	repo.getErr = apperrors.ErrNotFound
	repo.getErrAfter = 2
	repo.getCalls = 0
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getErr = errors.New("db2")
	repo.getCalls = 0
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentUpload, multipartRequest(t, "/note/1/attachments", "attachment", "a.png", "hello"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getErr = nil
	repo.getErrAfter = 0
	storage.saveErr = nil

	// baixar anexo
	attachGet := func(noteID, attachID string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/note/"+noteID+"/attachments/"+attachID, nil)
		r.SetPathValue("id", noteID)
		r.SetPathValue("attachmentId", attachID)
		return r
	}
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDownload, attachGet("1", "5"))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))

	att := sampleAttachment(5)
	att.MimeType = textVal("application/pdf")
	repo.attachment = &att
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDownload, attachGet("1", "5"))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")

	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDownload, attachGet("x", "5"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDownload, attachGet("1", "x"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getAttachErr = apperrors.ErrNotFound
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDownload, attachGet("1", "5"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getAttachErr = errors.New("db")
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDownload, attachGet("1", "5"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.getAttachErr = nil
	storage.pathErr = errors.New("missing")
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDownload, attachGet("1", "5"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	storage.pathErr = nil

	// exclusão
	attachDel := func(noteID, attachID string, json bool) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/note/"+noteID+"/attachments/"+attachID+"/delete", nil)
		r.SetPathValue("id", noteID)
		r.SetPathValue("attachmentId", attachID)
		if json {
			r.Header.Set("Accept", "application/json")
		}
		return r
	}
	repo.deletedAttach = ptrAttach(sampleAttachment(5))
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDelete, attachDel("1", "5", false))
	assert.Equal(t, http.StatusNoContent, rec.Code)

	storage.deleteErr = errors.New("disk")
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDelete, attachDel("1", "5", true))
	assert.Equal(t, http.StatusOK, rec.Code)

	noKey := sampleAttachment(5)
	noKey.StorageKey = pgtype.Text{Valid: false}
	repo.deletedAttach = &noKey
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDelete, attachDel("1", "5", true))
	assert.Equal(t, http.StatusOK, rec.Code)

	repo.deleteAttachErr = apperrors.ErrNotFound
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDelete, attachDel("1", "5", false))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.deleteAttachErr = errors.New("db")
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDelete, attachDel("1", "5", false))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	repo.deleteAttachErr = nil

	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDelete, attachDel("x", "5", false))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveNoteHandler(t, session, 1, h.NoteAttachmentDelete, attachDel("1", "x", false))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	assert.Contains(t, apperrors.AttachmentUploadMessage(apperrors.ErrAttachmentTooLarge), "10 MB")
	assert.Contains(t, apperrors.AttachmentUploadMessage(apperrors.ErrAttachmentTypeNotAllowed), "não permitido")
	assert.Contains(t, apperrors.AttachmentUploadMessage(apperrors.ErrAttachmentEmpty), "vazio")
	assert.Contains(t, apperrors.AttachmentUploadMessage(apperrors.ErrAttachmentInvalidName), "inválido")
	assert.Contains(t, apperrors.AttachmentUploadMessage(apperrors.ErrAttachmentMalicious), "bloqueado")
	assert.Contains(t, apperrors.AttachmentUploadMessage(apperrors.ErrAttachmentScanFailed), "verificar")
	assert.Contains(t, apperrors.AttachmentUploadMessage(errors.New("other")), "Tente novamente")
	assert.True(t, noteAttachmentCanRenderInline("IMAGE/JPEG"))
	assert.False(t, noteAttachmentCanRenderInline("text/plain"))
}

func ptrNote(n notemodel.Note) *notemodel.Note { return &n }
func ptrAttach(a notemodel.NoteAttachment) *notemodel.NoteAttachment {
	return &a
}
