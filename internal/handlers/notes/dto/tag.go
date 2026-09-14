package dto

import (
	"strconv"
	"strings"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

func NewNoteTagResponse(tag models.NoteTag) NoteTagResponse {
	return NoteTagResponse{
		Id:    numericToInt(tag.Id),
		Name:  tag.Name.String,
		Slug:  tag.Slug.String,
		Color: sanitizeOptionalHexColor(tag.Color.String),
	}
}

func NewNoteTagResponseList(tags []models.NoteTag) []NoteTagResponse {
	res := make([]NoteTagResponse, 0, len(tags))
	for _, tag := range tags {
		if !tag.Name.Valid || strings.TrimSpace(tag.Name.String) == "" {
			continue
		}
		res = append(res, NewNoteTagResponse(tag))
	}
	return res
}

func NewSelectableNoteTagResponseList(tags []models.NoteTag, selectedIDs []int) []NoteTagResponse {
	selected := intSet(selectedIDs)
	res := NewNoteTagResponseList(tags)
	for i := range res {
		res[i].Selected = selected[res[i].Id]
	}
	return res
}

func ParseNoteTagIDs(values []string) []int {
	ids := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func ValidateNoteTagSelection(selectedIDs []int, availableTags []models.NoteTag) ([]int, string) {
	selectedIDs = validations.NormalizeTagIDs(selectedIDs)
	if len(selectedIDs) > validations.MaxNoteTags {
		return selectedIDs, "Use no máximo 8 etiquetas por nota"
	}

	available := map[int]bool{}
	for _, tag := range availableTags {
		available[numericToInt(tag.Id)] = true
	}
	for _, id := range selectedIDs {
		if !available[id] {
			return selectedIDs, "Uma ou mais etiquetas selecionadas não existem"
		}
	}
	return selectedIDs, ""
}

func NoteTagIDsFromModels(tags []models.NoteTag) []int {
	ids := make([]int, 0, len(tags))
	for _, tag := range tags {
		id := numericToInt(tag.Id)
		if id > 0 {
			ids = append(ids, id)
		}
	}
	return validations.NormalizeTagIDs(ids)
}

func NewNoteTagForm(id int, name, color, redirect string) NoteTagForm {
	return NoteTagForm{
		Id:       id,
		Name:     validations.NormalizeTagName(name),
		Color:    sanitizeOptionalHexColor(color),
		Redirect: strings.TrimSpace(redirect),
	}
}

func ValidateNoteTagForm(id int, name, color, redirect string) NoteTagForm {
	form := NewNoteTagForm(id, name, color, redirect)
	if form.Name == "" {
		form.AddFieldError("name", "Nome da etiqueta é obrigatório")
	} else if len([]rune(form.Name)) > validations.MaxNoteTagNameLen {
		form.AddFieldError("name", "A etiqueta deve ter no máximo 32 caracteres")
	}
	if strings.TrimSpace(color) != "" && form.Color == "" {
		form.AddFieldError("color", "Use uma cor hexadecimal válida")
	}
	return form
}

func NewNoteTagsPage(tags []models.NoteTag, form NoteTagForm) NoteTagsPage {
	if form.Color == "" {
		form.Color = "#2f4538"
	}
	return NoteTagsPage{
		Tags: NewNoteTagResponseList(tags),
		Form: form,
	}
}

func intSet(ids []int) map[int]bool {
	res := make(map[int]bool, len(ids))
	for _, id := range ids {
		if id > 0 {
			res[id] = true
		}
	}
	return res
}
