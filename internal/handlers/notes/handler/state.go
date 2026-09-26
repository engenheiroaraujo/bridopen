package handlers

import (
	"net/http"
)

func (nh *noteHandler) NotePin(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := nh.svc.SetPinned(r.Context(), int(nh.getUserIdFromSession(r)), id, true); err != nil {
		return err
	}
	http.Redirect(w, r, noteActionRedirect(r, "/"), http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteUnpin(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := nh.svc.SetPinned(r.Context(), int(nh.getUserIdFromSession(r)), id, false); err != nil {
		return err
	}
	http.Redirect(w, r, noteActionRedirect(r, "/"), http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteArchive(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := nh.svc.Archive(r.Context(), int(nh.getUserIdFromSession(r)), id); err != nil {
		return err
	}
	http.Redirect(w, r, noteActionRedirect(r, "/"), http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteUnarchive(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := nh.svc.Unarchive(r.Context(), int(nh.getUserIdFromSession(r)), id); err != nil {
		return err
	}
	http.Redirect(w, r, noteActionRedirect(r, "/notes/archive"), http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteRestore(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := nh.svc.Restore(r.Context(), int(nh.getUserIdFromSession(r)), id); err != nil {
		return err
	}
	http.Redirect(w, r, noteActionRedirect(r, "/notes/trash"), http.StatusSeeOther)
	return nil
}

func (nh *noteHandler) NoteDeletePermanently(w http.ResponseWriter, r *http.Request) error {
	id, err := noteIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := nh.svc.DeletePermanently(r.Context(), int(nh.getUserIdFromSession(r)), id); err != nil {
		return err
	}
	if noteWantsJSON(r) {
		return writeNoteActionJSON(w, http.StatusOK, noteActionJSONResponse{
			OK:       true,
			Message:  "Nota excluída definitivamente.",
			Redirect: "/notes/trash",
		})
	}
	http.Redirect(w, r, "/notes/trash", http.StatusSeeOther)
	return nil
}
