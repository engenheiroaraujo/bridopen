package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"
	notedto "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/dto"
	noteservice "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/service"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/render"
	"github.com/gorilla/csrf"
)

type noteHandler struct {
	render  *render.RenderTemplate
	session *scs.SessionManager
	svc     *noteservice.NoteService
}

func NewNoteHandler(render *render.RenderTemplate, session *scs.SessionManager, svc *noteservice.NoteService) *noteHandler {
	return &noteHandler{render: render, session: session, svc: svc}
}

func (nh *noteHandler) getUserIdFromSession(r *http.Request) int64 {
	return nh.session.GetInt64(r.Context(), "userId")
}

func (nh *noteHandler) NoteList(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path != "/" {
		return apperrors.ErrPageNotFound
	}
	userID := int(nh.getUserIdFromSession(r))
	filter := noteListFilterFromRequest(r)
	result, err := nh.svc.List(r.Context(), userID, filter)
	if err != nil {
		return err
	}
	page := notedto.NewNoteListPage(result.Notes, result.Colors, result.Tags, filter.Search, filter.Color, filter.Tag, filter.Sort, filter.Pinned)
	page.ActionRedirect = r.URL.RequestURI()
	return nh.render.RenderPage(w, r, http.StatusOK, "home.html", page)
}

func (nh *noteHandler) NoteView(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return apperrors.ErrPageNotFound
	}
	// contexto para resposta demorada do banco de dados, desta froma aborta para não consumir recursos
	// ctx, cancel := context.WithTimeout(r.Context(), int(nh.getUserIdFromSession(r)), time.Minute)
	// defer cancel()

	note, err := nh.svc.Get(r.Context(), int(nh.getUserIdFromSession(r)), id)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrPageNotFound
		}
		return err
	}
	page := notedto.NewNoteResponseFromNote(note)
	page.AttachmentRedirect = fmt.Sprintf("/note/%d", id)
	return nh.render.RenderPage(w, r, http.StatusOK, "note-view.html", page)
}

func (nh *noteHandler) NoteNew(w http.ResponseWriter, r *http.Request) error {
	tags, err := nh.svc.ListTags(r.Context(), int(nh.getUserIdFromSession(r)))
	if err != nil {
		return err
	}
	data := notedto.NewNoteRequest(0, "", "", "", nil, tags)
	data.CSRFField = csrf.TemplateField(r)
	return nh.render.RenderPage(w, r, http.StatusOK, "note-new.html", data)
}

func (nh *noteHandler) NoteSave(w http.ResponseWriter, r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}
	idParam := r.PostForm.Get("id")
	id, _ := strconv.Atoi(strings.TrimSpace(idParam))
	title := strings.TrimSpace(r.PostForm.Get("title"))
	content := strings.TrimSpace(r.PostForm.Get("content"))
	color := r.PostForm.Get("color")
	tagIDs := notedto.ParseNoteTagIDs(r.PostForm["tag_ids"])

	result, err := nh.svc.Save(r.Context(), int(nh.getUserIdFromSession(r)), noteservice.SaveNoteInput{
		Id:      id,
		Title:   title,
		Content: content,
		Color:   color,
		TagIDs:  tagIDs,
	})
	if err != nil {
		return err
	}

	if len(result.FieldErrors) > 0 {
		data := notedto.NewNoteRequest(id, title, content, color, result.TagIDs, result.AvailableTags)
		data.CSRFField = csrf.TemplateField(r)
		for field, message := range result.FieldErrors {
			data.AddFieldError(field, message)
		}
		if id > 0 {
			return nh.render.RenderPage(w, r, http.StatusUnprocessableEntity, "note-edit.html", data)
		}
		return nh.render.RenderPage(w, r, http.StatusUnprocessableEntity, "note-new.html", data)
	}

	noteID := ""
	if result.Note.Id.Int != nil {
		noteID = result.Note.Id.Int.String()
	}
	http.Redirect(w, r, fmt.Sprintf("/note/%s", noteID), http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteEdit(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return apperrors.ErrPageNotFound
	}
	note, err := nh.svc.Get(r.Context(), int(nh.getUserIdFromSession(r)), id)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrPageNotFound
		}
		return err
	}
	tags, err := nh.svc.ListTags(r.Context(), int(nh.getUserIdFromSession(r)))
	if err != nil {
		return err
	}
	data := notedto.NewNoteRequestFromNote(note, tags)
	data.CSRFField = csrf.TemplateField(r)
	data.AttachmentRedirect = fmt.Sprintf("/note/%d/edit", id)
	return nh.render.RenderPage(w, r, http.StatusOK, "note-edit.html", data)
}

func (nh *noteHandler) NoteDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := nh.svc.MoveToTrash(r.Context(), int(nh.getUserIdFromSession(r)), id); err != nil {
		return err
	}
	if noteWantsJSON(r) {
		return writeNoteActionJSON(w, http.StatusOK, noteActionJSONResponse{
			OK:       true,
			Message:  "Nota movida para a lixeira.",
			Redirect: "/",
		})
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteArchiveList(w http.ResponseWriter, r *http.Request) error {
	notes, err := nh.svc.ListArchived(r.Context(), int(nh.getUserIdFromSession(r)))
	if err != nil {
		return err
	}
	return nh.render.RenderPage(w, r, http.StatusOK, "notes-archive.html", notedto.NewNoteResponseFromNoteList(notes))
}

func (nh *noteHandler) NoteTrashList(w http.ResponseWriter, r *http.Request) error {
	notes, err := nh.svc.ListDeleted(r.Context(), int(nh.getUserIdFromSession(r)))
	if err != nil {
		return err
	}
	return nh.render.RenderPage(w, r, http.StatusOK, "notes-trash.html", notedto.NewNoteResponseFromNoteList(notes))
}
