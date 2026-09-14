package handlers

import (
	"net/http"
	"strings"

	notemodel "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

func noteListFilterFromRequest(r *http.Request) notemodel.NoteFilter {
	query := r.URL.Query()
	rawSearch := validations.TruncateRunes(strings.TrimSpace(query.Get("q")), 120)
	color := sanitizeNoteColorFilter(query.Get("color"))
	tag := sanitizeNoteTagFilter(query.Get("tag"))
	pinned := strings.EqualFold(strings.TrimSpace(query.Get("pinned")), "true")

	search, parsed := parseNoteSearchDirectives(rawSearch)
	if tag == "" {
		tag = parsed.Tag
	}
	if color == "" {
		color = parsed.Color
	}
	pinned = pinned || parsed.Pinned

	sort := strings.TrimSpace(query.Get("sort"))
	switch sort {
	case notemodel.NoteSortRecent, notemodel.NoteSortOldest, notemodel.NoteSortPinned, notemodel.NoteSortRelevance:
	default:
		sort = ""
	}
	if sort == "" || (sort == notemodel.NoteSortRelevance && search == "") {
		if search != "" {
			sort = notemodel.NoteSortRelevance
		} else {
			sort = notemodel.NoteSortPinned
		}
	}

	return notemodel.NoteFilter{
		Search: search,
		Color:  color,
		Tag:    tag,
		Sort:   sort,
		Pinned: pinned,
	}
}

type parsedNoteSearchDirectives struct {
	Color  string
	Tag    string
	Pinned bool
}

func parseNoteSearchDirectives(search string) (string, parsedNoteSearchDirectives) {
	var parsed parsedNoteSearchDirectives
	terms := strings.Fields(strings.TrimSpace(search))
	if len(terms) == 0 {
		return "", parsed
	}

	kept := make([]string, 0, len(terms))
	for _, term := range terms {
		clean := strings.TrimSpace(term)
		lower := strings.ToLower(clean)
		switch {
		case lower == "fixadas" || lower == "fixada" || lower == "pinned":
			parsed.Pinned = true
		case strings.HasPrefix(lower, "cor:") || strings.HasPrefix(lower, "color:"):
			value := clean[strings.Index(clean, ":")+1:]
			if color := sanitizeNoteColorFilter(value); color != "" {
				parsed.Color = color
				continue
			}
			kept = append(kept, clean)
		case strings.HasPrefix(clean, "#") && len([]rune(clean)) > 1:
			if tag := sanitizeNoteTagFilter(clean); tag != "" {
				parsed.Tag = tag
				continue
			}
			kept = append(kept, clean)
		default:
			kept = append(kept, clean)
		}
	}
	return validations.TruncateRunes(strings.Join(kept, " "), 120), parsed
}

func sanitizeNoteTagFilter(tag string) string {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
	return validations.TruncateRunes(validations.NormalizeTagSlug(tag), 80)
}

func sanitizeNoteColorFilter(color string) string {
	if !validations.IsHexColor(color) {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(color))
}
