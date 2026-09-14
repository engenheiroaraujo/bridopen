package handlers

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"

	notedto "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/dto"
	noteservice "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/service"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/gorilla/csrf"
)

func (nh *noteHandler) NoteAttachmentUpload(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	userID := int(nh.getUserIdFromSession(r))
	redirect := fmt.Sprintf("/note/%d/edit", id)
	r.Body = http.MaxBytesReader(w, r.Body, noteservice.MaxAttachmentSizeBytes+(1<<20))
	if err := r.ParseMultipartForm(noteservice.MaxAttachmentSizeBytes + (1 << 20)); err != nil {
		return nh.renderNoteAttachmentError(w, r, http.StatusRequestEntityTooLarge, id, redirect, "O arquivo excede o limite permitido. Envie um arquivo com até 10 MB.")
	}
	redirect = noteActionRedirect(r, redirect)

	// A nota é carregada antes do arquivo para preservar a ordem das mensagens:
	// nota inexistente e nota na lixeira respondem antes de "selecione um arquivo".
	note, err := nh.svc.Get(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrPageNotFound
		}
		return err
	}
	if note.DeletedAt.Valid {
		return nh.renderNoteAttachmentError(w, r, http.StatusUnprocessableEntity, id, redirect, "Restaure a nota antes de adicionar anexos.")
	}

	file, header, err := r.FormFile("attachment")
	if err != nil {
		return nh.renderNoteAttachmentError(w, r, http.StatusUnprocessableEntity, id, redirect, "Selecione um arquivo para anexar.")
	}
	defer file.Close()

	_, err = nh.svc.UploadAttachment(r.Context(), userID, id, file, header)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrAttachmentLimit):
			return nh.renderNoteAttachmentError(w, r, http.StatusUnprocessableEntity, id, redirect, "Limite de 10 anexos por nota atingido.")
		case errors.Is(err, apperrors.ErrNotFound):
			return apperrors.ErrPageNotFound
		case errors.Is(err, apperrors.ErrNoteInTrash):
			return nh.renderNoteAttachmentError(w, r, http.StatusUnprocessableEntity, id, redirect, "Restaure a nota antes de adicionar anexos.")
		case errors.Is(err, apperrors.ErrAttachmentSaveFailed):
			return nh.renderNoteAttachmentError(w, r, http.StatusUnprocessableEntity, id, redirect, apperrors.AttachmentUploadMessage(err))
		}
		// falha de banco ao registrar o anexo: erro interno, não de formulário.
		return err
	}

	http.Redirect(w, r, redirect, http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteAttachmentDownload(w http.ResponseWriter, r *http.Request) error {
	noteID, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	attachmentID, err := noteAttachmentIDFromRequest(r)
	if err != nil {
		return err
	}
	userID := int(nh.getUserIdFromSession(r))
	attachment, filePath, err := nh.svc.GetAttachment(r.Context(), userID, noteID, attachmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrPageNotFound
		}
		if attachment != nil {
			// caminho físico inválido: trata como recurso ausente, igual ao fluxo anterior.
			return apperrors.ErrPageNotFound
		}
		return err
	}

	disposition := "attachment"
	if noteAttachmentCanRenderInline(attachment.MimeType.String) {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", attachment.MimeType.String)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": attachment.OriginalName.String}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filePath)
	return nil
}

func (nh *noteHandler) NoteAttachmentDelete(w http.ResponseWriter, r *http.Request) error {
	noteID, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	attachmentID, err := noteAttachmentIDFromRequest(r)
	if err != nil {
		return err
	}
	userID := int(nh.getUserIdFromSession(r))
	if err := nh.svc.DeleteAttachment(r.Context(), userID, noteID, attachmentID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrPageNotFound
		}
		return err
	}
	if noteWantsJSON(r) {
		return writeNoteActionJSON(w, http.StatusOK, noteActionJSONResponse{
			OK:      true,
			Message: "Anexo removido.",
			Reload:  true,
		})
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (nh *noteHandler) renderNoteAttachmentError(w http.ResponseWriter, r *http.Request, status int, noteID int, redirect string, message string) error {
	userID := int(nh.getUserIdFromSession(r))
	note, err := nh.svc.Get(r.Context(), userID, noteID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrPageNotFound
		}
		return err
	}
	if strings.Contains(redirect, "/edit") && !note.DeletedAt.Valid {
		tags, err := nh.svc.ListTags(r.Context(), userID)
		if err != nil {
			return err
		}
		data := notedto.NewNoteRequestFromNote(note, tags)
		data.CSRFField = csrf.TemplateField(r)
		data.AttachmentRedirect = redirect
		data.AttachmentError = message
		return nh.render.RenderPage(w, r, status, "note-edit.html", data)
	}
	page := notedto.NewNoteResponseFromNote(note)
	page.AttachmentRedirect = redirect
	page.AttachmentError = message
	return nh.render.RenderPage(w, r, status, "note-view.html", page)
}

func noteAttachmentCanRenderInline(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return strings.HasPrefix(mimeType, "image/")
}
