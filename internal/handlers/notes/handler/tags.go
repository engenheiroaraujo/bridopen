package handlers

import (
	"errors"
	"net/http"

	notedto "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/dto"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
)

func (nh *noteHandler) renderNoteTagsPage(w http.ResponseWriter, r *http.Request, status int, form notedto.NoteTagForm) error {
	tags, err := nh.svc.ListTags(r.Context(), int(nh.getUserIdFromSession(r)))
	if err != nil {
		return err
	}
	return nh.render.RenderPage(w, r, status, "note-tags.html", notedto.NewNoteTagsPage(tags, form))
}

func (nh *noteHandler) NoteTagList(w http.ResponseWriter, r *http.Request) error {
	return nh.renderNoteTagsPage(w, r, http.StatusOK, notedto.NewNoteTagForm(0, "", "#2f4538", ""))
}

func (nh *noteHandler) NoteTagCreate(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	form := notedto.ValidateNoteTagForm(0, r.PostFormValue("name"), r.PostFormValue("color"), r.PostFormValue("redirect"))
	if !form.Valid() {
		nh.svc.RecordTagValidationFailure(r.Context(), nh.getUserIdFromSession(r), 0)
		if noteTagWantsJSON(r) {
			return writeNoteTagJSON(w, http.StatusUnprocessableEntity, noteTagJSONResponse{OK: false, FieldErrors: form.FieldErrors})
		}
		return nh.renderNoteTagsPage(w, r, http.StatusUnprocessableEntity, form)
	}

	tag, err := nh.svc.CreateTag(r.Context(), int(nh.getUserIdFromSession(r)), form.Name, form.Color)
	if err != nil {
		if errors.Is(err, apperrors.ErrDuplicate) {
			form.AddFieldError("name", "Já existe uma etiqueta com esse nome")
			nh.svc.RecordTagValidationFailure(r.Context(), nh.getUserIdFromSession(r), 0)
			if noteTagWantsJSON(r) {
				return writeNoteTagJSON(w, http.StatusConflict, noteTagJSONResponse{OK: false, FieldErrors: form.FieldErrors})
			}
			return nh.renderNoteTagsPage(w, r, http.StatusUnprocessableEntity, form)
		}
		return err
	}

	if noteTagWantsJSON(r) {
		return writeNoteTagJSON(w, http.StatusCreated, noteTagJSONResponse{OK: true, Tag: notedto.NewNoteTagResponse(*tag)})
	}

	http.Redirect(w, r, noteActionRedirect(r, "/tags"), http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteTagUpdate(w http.ResponseWriter, r *http.Request) error {
	id, err := noteTagIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	form := notedto.ValidateNoteTagForm(id, r.PostFormValue("name"), r.PostFormValue("color"), r.PostFormValue("redirect"))
	if !form.Valid() {
		nh.svc.RecordTagValidationFailure(r.Context(), nh.getUserIdFromSession(r), int64(id))
		return nh.renderNoteTagsPage(w, r, http.StatusUnprocessableEntity, form)
	}

	_, err = nh.svc.UpdateTag(r.Context(), int(nh.getUserIdFromSession(r)), id, form.Name, form.Color)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrPageNotFound
		}
		if errors.Is(err, apperrors.ErrDuplicate) {
			form.AddFieldError("name", "Já existe uma etiqueta com esse nome")
			nh.svc.RecordTagValidationFailure(r.Context(), nh.getUserIdFromSession(r), int64(id))
			return nh.renderNoteTagsPage(w, r, http.StatusUnprocessableEntity, form)
		}
		return err
	}

	http.Redirect(w, r, noteActionRedirect(r, "/tags"), http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteTagDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := noteTagIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := r.ParseForm(); err != nil {
		return err
	}

	if err := nh.svc.DeleteTag(r.Context(), int(nh.getUserIdFromSession(r)), id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrPageNotFound
		}
		return err
	}

	http.Redirect(w, r, noteActionRedirect(r, "/tags"), http.StatusSeeOther)
	return nil
}
