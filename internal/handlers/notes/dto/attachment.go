package dto

import (
	"strconv"
	"strings"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
)

func NewNoteAttachmentResponse(attachment models.NoteAttachment, noteID int) NoteAttachmentResponse {
	name := strings.TrimSpace(attachment.OriginalName.String)
	mimeType := strings.TrimSpace(attachment.MimeType.String)
	id := numericToInt(attachment.Id)
	return NoteAttachmentResponse{
		Id:        id,
		Name:      name,
		MimeType:  mimeType,
		SizeLabel: attachmentSizeLabel(attachment.SizeBytes.Int64),
		URL:       "/note/" + strconv.Itoa(noteID) + "/attachments/" + strconv.Itoa(id),
		IsImage:   strings.HasPrefix(mimeType, "image/"),
	}
}

func NewNoteAttachmentResponseList(attachments []models.NoteAttachment, noteID int) []NoteAttachmentResponse {
	res := make([]NoteAttachmentResponse, 0, len(attachments))
	for _, attachment := range attachments {
		if !attachment.OriginalName.Valid || strings.TrimSpace(attachment.OriginalName.String) == "" {
			continue
		}
		res = append(res, NewNoteAttachmentResponse(attachment, noteID))
	}
	return res
}

func attachmentSizeLabel(size int64) string {
	switch {
	case size >= 1<<20:
		return strconv.FormatFloat(float64(size)/(1<<20), 'f', 1, 64) + " MB"
	case size >= 1<<10:
		return strconv.FormatFloat(float64(size)/(1<<10), 'f', 1, 64) + " KB"
	case size == 1:
		return "1 byte"
	default:
		return strconv.FormatInt(size, 10) + " bytes"
	}
}
