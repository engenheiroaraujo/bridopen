package dto

import (
	"net/url"
	"strconv"
	"strings"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
)

func NewNoteListPage(notes []models.Note, colors []string, tags []models.NoteTag, search, color, tag, sort string, pinnedOnly bool) NoteListPage {
	color = sanitizeOptionalHexColor(color)
	search = strings.TrimSpace(search)
	tag = strings.TrimSpace(tag)
	availableTags := NewNoteTagResponseList(tags)
	return NoteListPage{
		Notes:           NewNoteResponseFromNoteList(notes),
		Search:          search,
		Color:           color,
		Tag:             tag,
		Sort:            strings.TrimSpace(sort),
		PinnedOnly:      pinnedOnly,
		AvailableColors: NewNoteColorList(colors),
		AvailableTags:   availableTags,
		HasFilters:      search != "" || color != "" || tag != "" || pinnedOnly,
		HasSearch:       search != "",
		ResultCount:     len(notes),
		ResultSummary:   noteResultSummary(len(notes), search != "" || color != "" || tag != "" || pinnedOnly),
		FilterChips:     noteFilterChips(search, color, tag, sort, pinnedOnly, availableTags),
		SearchHint:      noteSearchHint(search, tag, color, pinnedOnly),
	}
}

func noteResultSummary(count int, filtered bool) string {
	if count == 1 {
		if filtered {
			return "1 nota encontrada"
		}
		return "1 nota ativa"
	}
	if filtered {
		return strconv.Itoa(count) + " notas encontradas"
	}
	return strconv.Itoa(count) + " notas ativas"
}

func noteSearchHint(search, tag, color string, pinnedOnly bool) string {
	parts := []string{}
	if search != "" {
		parts = append(parts, `texto "`+search+`"`)
	}
	if tag != "" {
		parts = append(parts, "#"+tag)
	}
	if color != "" {
		parts = append(parts, "cor "+color)
	}
	if pinnedOnly {
		parts = append(parts, "fixadas")
	}
	if len(parts) == 0 {
		return "Use texto, #etiqueta, cor:#fff ou fixadas."
	}
	return "Filtrando por " + strings.Join(parts, ", ") + "."
}

func noteFilterChips(search, color, tag, sort string, pinnedOnly bool, tags []NoteTagResponse) []NoteFilterChip {
	chips := []NoteFilterChip{}
	if search != "" {
		chips = append(chips, NoteFilterChip{
			Label: "Texto",
			Value: search,
			Href:  noteFilterURL("", color, tag, sort, pinnedOnly),
		})
	}
	if color != "" {
		chips = append(chips, NoteFilterChip{
			Label: "Cor",
			Value: color,
			Href:  noteFilterURL(search, "", tag, sort, pinnedOnly),
		})
	}
	if tag != "" {
		chips = append(chips, NoteFilterChip{
			Label: "Etiqueta",
			Value: "#" + noteTagNameForSlug(tag, tags),
			Href:  noteFilterURL(search, color, "", sort, pinnedOnly),
		})
	}
	if pinnedOnly {
		chips = append(chips, NoteFilterChip{
			Label: "Estado",
			Value: "Fixadas",
			Href:  noteFilterURL(search, color, tag, sort, false),
		})
	}
	return chips
}

func noteTagNameForSlug(slug string, tags []NoteTagResponse) string {
	for _, tag := range tags {
		if tag.Slug == slug && strings.TrimSpace(tag.Name) != "" {
			return tag.Name
		}
	}
	return slug
}

func noteFilterURL(search, color, tag, sort string, pinnedOnly bool) string {
	values := url.Values{}
	if strings.TrimSpace(search) != "" {
		values.Set("q", strings.TrimSpace(search))
	}
	if color = sanitizeOptionalHexColor(color); color != "" {
		values.Set("color", color)
	}
	if strings.TrimSpace(tag) != "" {
		values.Set("tag", strings.TrimSpace(tag))
	}
	if pinnedOnly {
		values.Set("pinned", "true")
	}
	if strings.TrimSpace(sort) != "" {
		values.Set("sort", strings.TrimSpace(sort))
	}
	encoded := values.Encode()
	if encoded == "" {
		return "/"
	}
	return "/?" + encoded
}

func NewNoteColorList(colors []string) []string {
	seen := make(map[string]bool, len(colors))
	list := make([]string, 0, len(colors))
	for _, color := range colors {
		color = sanitizeOptionalHexColor(color)
		if color == "" || seen[color] {
			continue
		}
		seen[color] = true
		list = append(list, color)
	}
	return list
}
