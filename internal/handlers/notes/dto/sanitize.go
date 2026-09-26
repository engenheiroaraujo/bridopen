package dto

import (
	"strings"

	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
	"github.com/jackc/pgx/v5/pgtype"
)

// Cor padrão estilo Google Keep quando a coluna color vier vazia ou inválida.
const keepDefaultNoteColor = "#FFF475"

func sanitizeHexColor(s string) string {
	if !validations.IsHexColor(s) {
		return keepDefaultNoteColor
	}
	return strings.TrimSpace(s)
}

func sanitizeOptionalHexColor(s string) string {
	if !validations.IsHexColor(s) {
		return ""
	}
	return strings.TrimSpace(s)
}

func numericToInt(value pgtype.Numeric) int {
	if !value.Valid || value.Int == nil {
		return 0
	}
	return int(value.Int.Int64())
}
