package dto

import (
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
	"math/big"
	"strings"
	"testing"
	"time"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func numeric(n int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(n), Valid: true}
}

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

func sampleNote(t *testing.T) models.Note {
	t.Helper()
	archived := pgtype.Timestamp{Time: time.Now(), Valid: true}
	return models.Note{
		Id:         numeric(10),
		Title:      text("Título"),
		Content:    text("Conteúdo"),
		Color:      text("#aabbcc"),
		Pinned:     pgtype.Bool{Bool: true, Valid: true},
		ArchivedAt: archived,
		Tags: []models.NoteTag{
			{Id: numeric(1), Name: text("Trabalho"), Slug: text("trabalho"), Color: text("#112233")},
			{Id: numeric(2), Name: pgtype.Text{Valid: false}, Slug: text("skip")},
			{Id: numeric(3), Name: text("   "), Slug: text("blank")},
		},
		Attachments: []models.NoteAttachment{
			{
				Id:           numeric(5),
				OriginalName: text("foto.png"),
				MimeType:     text("image/png"),
				SizeBytes:    pgtype.Int8{Int64: 1 << 20, Valid: true},
			},
			{
				Id:           numeric(6),
				OriginalName: pgtype.Text{Valid: false},
				MimeType:     text("text/plain"),
			},
			{
				Id:           numeric(7),
				OriginalName: text("  "),
				MimeType:     text("text/plain"),
			},
		},
	}
}

// TestNewNoteListPageBuildsFilterChipsAndSummary verifica que NewNoteListPage monta chips de filtro, resumo e dica de busca com filtros ativos.
func TestNewNoteListPageBuildsFilterChipsAndSummary(t *testing.T) {
	t.Parallel()

	tags := []models.NoteTag{
		{
			Name: pgtype.Text{String: "Trabalho", Valid: true},
			Slug: pgtype.Text{String: "trabalho", Valid: true},
		},
	}

	page := NewNoteListPage(nil, []string{"#aabbcc"}, tags, "reuniao", "#aabbcc", "trabalho", "relevance", true)

	require.True(t, page.HasFilters)
	assert.Equal(t, "0 notas encontradas", page.ResultSummary)
	require.Len(t, page.FilterChips, 4)
	assert.Equal(t, "#Trabalho", page.FilterChips[2].Value)
	assert.True(t, page.HasSearch)
	assert.Equal(t, 0, page.ResultCount)
	assert.Contains(t, page.SearchHint, `texto "reuniao"`)
	assert.Equal(t, "#aabbcc", page.Color)
	assert.Equal(t, "relevance", page.Sort)
	assert.True(t, page.PinnedOnly)
}

// TestNewNoteListPageUnfilteredSingleAndMany verifica resumos sem filtro para uma ou muitas notas e a sanitização das cores disponíveis.
func TestNewNoteListPageUnfilteredSingleAndMany(t *testing.T) {
	t.Parallel()

	one := []models.Note{{Id: numeric(1), Title: text("Uma"), Color: text("#fff")}}
	pageOne := NewNoteListPage(one, nil, nil, "  ", "", "  ", "  ", false)
	assert.False(t, pageOne.HasFilters)
	assert.False(t, pageOne.HasSearch)
	assert.Equal(t, "1 nota ativa", pageOne.ResultSummary)
	assert.Equal(t, "Use texto, #etiqueta, cor:#fff ou fixadas.", pageOne.SearchHint)
	assert.Empty(t, pageOne.FilterChips)

	many := []models.Note{one[0], {Id: numeric(2), Title: text("Duas"), Color: text("#000")}}
	pageMany := NewNoteListPage(many, []string{"bad", "#abc", "#abc", "#zzzzzz"}, nil, "", "", "", "", false)
	assert.Equal(t, "2 notas ativas", pageMany.ResultSummary)
	assert.Equal(t, []string{"#abc"}, pageMany.AvailableColors)
}

// TestNewNoteListPageFilteredSingleNote verifica o resumo e a dica de busca quando há exatamente uma nota filtrada.
func TestNewNoteListPageFilteredSingleNote(t *testing.T) {
	t.Parallel()

	notes := []models.Note{{Id: numeric(1), Title: text("Uma"), Color: text("#fff")}}
	page := NewNoteListPage(notes, nil, nil, "q", "", "", "", false)
	assert.Equal(t, "1 nota encontrada", page.ResultSummary)
	assert.Equal(t, `Filtrando por texto "q".`, page.SearchHint)
}

// TestNewNoteResponseFromNote verifica a conversão de Note em NoteResponse, incluindo tags e anexos válidos.
func TestNewNoteResponseFromNote(t *testing.T) {
	t.Parallel()

	note := sampleNote(t)
	note.DeletedAt = pgtype.Timestamp{Time: time.Now(), Valid: true}
	res := NewNoteResponseFromNote(&note)

	assert.Equal(t, 10, res.Id)
	assert.Equal(t, "Título", res.Title)
	assert.Equal(t, "Conteúdo", res.Content)
	assert.Equal(t, "#aabbcc", res.Color)
	assert.True(t, res.Pinned)
	assert.True(t, res.Archived)
	assert.True(t, res.Deleted)
	require.Len(t, res.Tags, 1)
	assert.Equal(t, "Trabalho", res.Tags[0].Name)
	require.Len(t, res.Attachments, 1)
	assert.Equal(t, "foto.png", res.Attachments[0].Name)
	assert.True(t, res.Attachments[0].IsImage)
	assert.Equal(t, "1.0 MB", res.Attachments[0].SizeLabel)
	assert.Equal(t, "/note/10/attachments/5", res.Attachments[0].URL)
}

// TestNewNoteResponseFromNoteList verifica a conversão de uma lista de notas e o fallback de cor inválida.
func TestNewNoteResponseFromNoteList(t *testing.T) {
	t.Parallel()

	notes := []models.Note{
		{Id: numeric(1), Title: text("A"), Color: text("#111")},
		{Id: numeric(2), Title: text("B"), Color: text("invalid")},
	}
	res := NewNoteResponseFromNoteList(notes)
	require.Len(t, res, 2)
	assert.Equal(t, keepDefaultNoteColor, res[1].Color)
}

// TestNewNoteTagResponseAndList verifica o mapeamento de etiqueta e a filtragem de nomes inválidos na lista.
func TestNewNoteTagResponseAndList(t *testing.T) {
	t.Parallel()

	tag := models.NoteTag{
		Id:    numeric(9),
		Name:  text("Ideias"),
		Slug:  text("ideias"),
		Color: text("#FfEeDd"),
	}
	res := NewNoteTagResponse(tag)
	assert.Equal(t, 9, res.Id)
	assert.Equal(t, "Ideias", res.Name)
	assert.Equal(t, "ideias", res.Slug)
	assert.Equal(t, "#FfEeDd", res.Color)
	assert.False(t, res.Selected)

	invalidColor := NewNoteTagResponse(models.NoteTag{
		Id:    numeric(1),
		Name:  text("X"),
		Slug:  text("x"),
		Color: text("not-hex"),
	})
	assert.Empty(t, invalidColor.Color)

	list := NewNoteTagResponseList([]models.NoteTag{
		tag,
		{Name: pgtype.Text{Valid: false}},
		{Name: text("")},
		{Name: text("   ")},
	})
	require.Len(t, list, 1)
}

// TestNewNoteAttachmentResponse verifica a resposta de anexo (imagem/PDF), URL e rótulos de tamanho.
func TestNewNoteAttachmentResponse(t *testing.T) {
	t.Parallel()

	img := NewNoteAttachmentResponse(models.NoteAttachment{
		Id:           numeric(3),
		OriginalName: text("  pic.jpg "),
		MimeType:     text("  image/jpeg "),
		SizeBytes:    pgtype.Int8{Int64: 2048, Valid: true},
	}, 42)
	assert.Equal(t, 3, img.Id)
	assert.Equal(t, "pic.jpg", img.Name)
	assert.Equal(t, "image/jpeg", img.MimeType)
	assert.Equal(t, "2.0 KB", img.SizeLabel)
	assert.Equal(t, "/note/42/attachments/3", img.URL)
	assert.True(t, img.IsImage)

	doc := NewNoteAttachmentResponse(models.NoteAttachment{
		Id:           numeric(4),
		OriginalName: text("doc.pdf"),
		MimeType:     text("application/pdf"),
		SizeBytes:    pgtype.Int8{Int64: 1, Valid: true},
	}, 7)
	assert.False(t, doc.IsImage)
	assert.Equal(t, "1 byte", doc.SizeLabel)

	tiny := NewNoteAttachmentResponse(models.NoteAttachment{
		Id:           numeric(8),
		OriginalName: text("a.bin"),
		MimeType:     text("application/octet-stream"),
		SizeBytes:    pgtype.Int8{Int64: 0, Valid: true},
	}, 1)
	assert.Equal(t, "0 bytes", tiny.SizeLabel)
}

// TestAttachmentSizeLabel verifica a formatação legível de tamanhos de anexo (bytes, KB e MB).
func TestAttachmentSizeLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "1.5 MB", attachmentSizeLabel(int64(1.5*(1<<20))))
	assert.Equal(t, "1.5 KB", attachmentSizeLabel(int64(1.5*(1<<10))))
	assert.Equal(t, "1 byte", attachmentSizeLabel(1))
	assert.Equal(t, "42 bytes", attachmentSizeLabel(42))
	assert.Equal(t, "1024.0 KB", attachmentSizeLabel(1<<20-1))
}

// TestNewSelectableNoteTagResponseList verifica a marcação Selected das etiquetas selecionáveis.
func TestNewSelectableNoteTagResponseList(t *testing.T) {
	t.Parallel()

	tags := []models.NoteTag{
		{Id: numeric(1), Name: text("A"), Slug: text("a")},
		{Id: numeric(2), Name: text("B"), Slug: text("b")},
	}
	res := NewSelectableNoteTagResponseList(tags, []int{2, 0, -1, 2})
	require.Len(t, res, 2)
	assert.False(t, res[0].Selected)
	assert.True(t, res[1].Selected)
}

// TestNewNoteColorList verifica a deduplicação e validação da lista de cores hexadecimais.
func TestNewNoteColorList(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"#abc", "#AABBCC"}, NewNoteColorList([]string{
		"", "  ", "nope", "#abc", "#abc", "#AABBCC", "#12", "#ggg", "#12345g",
	}))
	assert.Empty(t, NewNoteColorList(nil))
}

// TestNewNoteRequestAndFromNote verifica a montagem de NoteRequest e a conversão a partir de Note.
func TestNewNoteRequestAndFromNote(t *testing.T) {
	t.Parallel()

	available := []models.NoteTag{
		{Id: numeric(1), Name: text("A"), Slug: text("a")},
		{Id: numeric(2), Name: text("B"), Slug: text("b")},
	}
	req := NewNoteRequest(5, "t", "c", "#fff", []int{1, 1, 0}, available)
	assert.Equal(t, 5, req.Id)
	assert.Equal(t, "t", req.Title)
	assert.Equal(t, "#fff", req.Color)
	assert.Equal(t, validations.MaxNoteTitleLen, req.MaxTitleLen)
	assert.Equal(t, []int{1}, req.SelectedTagIDs)
	require.Len(t, req.AvailableTags, 2)
	assert.True(t, req.AvailableTags[0].Selected)
	assert.False(t, req.AvailableTags[1].Selected)

	note := sampleNote(t)
	fromNote := NewNoteRequestFromNote(&note, available)
	assert.Equal(t, 10, fromNote.Id)
	assert.True(t, fromNote.Pinned)
	assert.True(t, fromNote.Archived)
	assert.False(t, fromNote.Deleted)
	require.NotEmpty(t, fromNote.Attachments)
	assert.Equal(t, []int{1, 2, 3}, fromNote.SelectedTagIDs)
}

// TestParseNoteTagIDs verifica o parse e a limpeza de IDs de etiquetas a partir de strings.
func TestParseNoteTagIDs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []int{1, 2}, ParseNoteTagIDs([]string{"1", " 2 ", "2", "0", "-3", "x", "", "  ", "3a"}))
	assert.Empty(t, ParseNoteTagIDs(nil))
}

// TestValidateNoteTagSelection verifica limites e existência na validação da seleção de etiquetas.
func TestValidateNoteTagSelection(t *testing.T) {
	t.Parallel()

	available := []models.NoteTag{
		{Id: numeric(1), Name: text("A")},
		{Id: numeric(2), Name: text("B")},
	}

	ids, errMsg := ValidateNoteTagSelection([]int{1, 2}, available)
	assert.Equal(t, []int{1, 2}, ids)
	assert.Empty(t, errMsg)

	tooMany := make([]int, validations.MaxNoteTags+1)
	for i := range tooMany {
		tooMany[i] = i + 1
	}
	ids, errMsg = ValidateNoteTagSelection(tooMany, available)
	assert.Equal(t, validations.MaxNoteTags+1, len(ids))
	assert.Equal(t, "Use no máximo 8 etiquetas por nota", errMsg)

	ids, errMsg = ValidateNoteTagSelection([]int{1, 99}, available)
	assert.Equal(t, []int{1, 99}, ids)
	assert.Equal(t, "Uma ou mais etiquetas selecionadas não existem", errMsg)
}

// TestNoteTagIDsFromModels verifica a extração de IDs válidos e únicos a partir dos modelos de etiqueta.
func TestNoteTagIDsFromModels(t *testing.T) {
	t.Parallel()

	ids := NoteTagIDsFromModels([]models.NoteTag{
		{Id: numeric(3)},
		{Id: numeric(0)},
		{Id: pgtype.Numeric{Valid: false}},
		{Id: numeric(3)},
		{Id: numeric(-1)},
	})
	assert.Equal(t, []int{3}, ids)
}

// TestNewNoteTagFormAndValidate verifica a criação e a validação do formulário de etiqueta.
func TestNewNoteTagFormAndValidate(t *testing.T) {
	t.Parallel()

	form := NewNoteTagForm(1, "  #Minha  Tag  ", "#abc", " /notes ")
	assert.Equal(t, 1, form.Id)
	assert.Equal(t, "Minha Tag", form.Name)
	assert.Equal(t, "#abc", form.Color)
	assert.Equal(t, "/notes", form.Redirect)

	empty := ValidateNoteTagForm(0, "   ", "", "")
	require.False(t, empty.Valid())
	assert.Equal(t, "Nome da etiqueta é obrigatório", empty.FieldErrors["name"])

	longName := strings.Repeat("á", validations.MaxNoteTagNameLen+1)
	long := ValidateNoteTagForm(0, longName, "", "")
	assert.Equal(t, "A etiqueta deve ter no máximo 32 caracteres", long.FieldErrors["name"])

	badColor := ValidateNoteTagForm(0, "ok", "azul", "")
	assert.Equal(t, "Use uma cor hexadecimal válida", badColor.FieldErrors["color"])

	ok := ValidateNoteTagForm(2, "ok", "#123abc", "/r")
	assert.True(t, ok.Valid())
	assert.Equal(t, "#123abc", ok.Color)

	emptyColorOk := ValidateNoteTagForm(0, "ok", "  ", "")
	assert.True(t, emptyColorOk.Valid())
	assert.Empty(t, emptyColorOk.Color)
}

// TestNewNoteTagsPage verifica a montagem da página de etiquetas e a cor padrão do formulário.
func TestNewNoteTagsPage(t *testing.T) {
	t.Parallel()

	tags := []models.NoteTag{{Id: numeric(1), Name: text("A"), Slug: text("a")}}
	page := NewNoteTagsPage(tags, NoteTagForm{Name: "x"})
	assert.Equal(t, "#2f4538", page.Form.Color)
	require.Len(t, page.Tags, 1)

	page2 := NewNoteTagsPage(nil, NoteTagForm{Color: "#abc"})
	assert.Equal(t, "#abc", page2.Form.Color)
	assert.Empty(t, page2.Tags)
}

// TestNormalizeNoteTagName verifica a normalização do nome da etiqueta (trim e remoção de #).
func TestNormalizeNoteTagName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Hello World", validations.NormalizeTagName("  #Hello   World  "))
	assert.Equal(t, "", validations.NormalizeTagName("   #   "))
	assert.Equal(t, "", validations.NormalizeTagName(""))
}

// TestNormalizeNoteTagIDsAndIntSet verifica a normalização de IDs e a construção do conjunto de inteiros.
func TestNormalizeNoteTagIDsAndIntSet(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []int{1, 2}, validations.NormalizeTagIDs([]int{1, 0, -2, 1, 2}))
	assert.Empty(t, validations.NormalizeTagIDs(nil))

	set := intSet([]int{1, 0, -1, 2, 1})
	assert.True(t, set[1])
	assert.True(t, set[2])
	assert.False(t, set[0])
}

// TestNumericToInt verifica a conversão segura de pgtype.Numeric para int.
func TestNumericToInt(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 42, numericToInt(numeric(42)))
	assert.Equal(t, 0, numericToInt(pgtype.Numeric{Valid: false}))
	assert.Equal(t, 0, numericToInt(pgtype.Numeric{Valid: true, Int: nil}))
}

// TestSanitizeHexColor verifica a sanitização de cor hexadecimal com fallback para a cor padrão.
func TestSanitizeHexColor(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "#abc", sanitizeHexColor("  #abc  "))
	assert.Equal(t, "#AABBCC", sanitizeHexColor("#AABBCC"))
	assert.Equal(t, keepDefaultNoteColor, sanitizeHexColor(""))
	assert.Equal(t, keepDefaultNoteColor, sanitizeHexColor("abc"))
	assert.Equal(t, keepDefaultNoteColor, sanitizeHexColor("#ab"))
	assert.Equal(t, keepDefaultNoteColor, sanitizeHexColor("#abcd"))
	assert.Equal(t, keepDefaultNoteColor, sanitizeHexColor("#ggg"))
	assert.Equal(t, keepDefaultNoteColor, sanitizeHexColor("#12345g"))
}

// TestSanitizeOptionalHexColor verifica a sanitização opcional de cor hexadecimal (vazio se inválida).
func TestSanitizeOptionalHexColor(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "#abc", sanitizeOptionalHexColor(" #abc "))
	assert.Equal(t, "#00ff00", sanitizeOptionalHexColor("#00ff00"))
	assert.Equal(t, "", sanitizeOptionalHexColor(""))
	assert.Equal(t, "", sanitizeOptionalHexColor("   "))
	assert.Equal(t, "", sanitizeOptionalHexColor("abc"))
	assert.Equal(t, "", sanitizeOptionalHexColor("#12"))
	assert.Equal(t, "", sanitizeOptionalHexColor("#1234"))
	assert.Equal(t, "", sanitizeOptionalHexColor("#xyz"))
	assert.Equal(t, "", sanitizeOptionalHexColor("#00ff0g"))
}

// TestNoteResultSummary verifica os textos de resumo para contagens filtradas e não filtradas.
func TestNoteResultSummary(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "1 nota encontrada", noteResultSummary(1, true))
	assert.Equal(t, "1 nota ativa", noteResultSummary(1, false))
	assert.Equal(t, "3 notas encontradas", noteResultSummary(3, true))
	assert.Equal(t, "0 notas ativas", noteResultSummary(0, false))
}

// TestNoteSearchHint verifica as dicas de busca com e sem filtros ativos.
func TestNoteSearchHint(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Use texto, #etiqueta, cor:#fff ou fixadas.", noteSearchHint("", "", "", false))
	assert.Equal(t, `Filtrando por texto "abc", #tag, cor #fff, fixadas.`, noteSearchHint("abc", "tag", "#fff", true))
	assert.Equal(t, "Filtrando por #tag.", noteSearchHint("", "tag", "", false))
	assert.Equal(t, "Filtrando por cor #abc.", noteSearchHint("", "", "#abc", false))
	assert.Equal(t, "Filtrando por fixadas.", noteSearchHint("", "", "", true))
}

// TestNoteFilterChipsAndURL verifica chips de filtro, nomes por slug e montagem das URLs de remoção.
func TestNoteFilterChipsAndURL(t *testing.T) {
	t.Parallel()

	tags := []NoteTagResponse{
		{Slug: "trabalho", Name: "Trabalho"},
		{Slug: "empty", Name: "   "},
	}
	chips := noteFilterChips("q", "#abc", "trabalho", "updated", true, tags)
	require.Len(t, chips, 4)
	assert.Equal(t, "Texto", chips[0].Label)
	assert.Equal(t, "/?color=%23abc&pinned=true&sort=updated&tag=trabalho", chips[0].Href)
	assert.Equal(t, "Cor", chips[1].Label)
	assert.Equal(t, "/?pinned=true&q=q&sort=updated&tag=trabalho", chips[1].Href)
	assert.Equal(t, "#Trabalho", chips[2].Value)
	assert.Equal(t, "/?color=%23abc&pinned=true&q=q&sort=updated", chips[2].Href)
	assert.Equal(t, "Fixadas", chips[3].Value)
	assert.Equal(t, "/?color=%23abc&q=q&sort=updated&tag=trabalho", chips[3].Href)

	assert.Equal(t, "missing", noteTagNameForSlug("missing", tags))
	assert.Equal(t, "empty", noteTagNameForSlug("empty", tags))

	assert.Equal(t, "/", noteFilterURL("", "", "", "", false))
	assert.Equal(t, "/?q=hi", noteFilterURL("  hi  ", "bad", "  ", "  ", false))
	assert.Equal(t, "/?color=%23fff&pinned=true&sort=title&tag=x", noteFilterURL("", "#fff", "x", "title", true))
	assert.Empty(t, noteFilterChips("", "", "", "", false, nil))
}
