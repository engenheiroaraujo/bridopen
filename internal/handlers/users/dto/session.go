package dto

import (
	"strings"
	"time"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
)

func NewActiveSessionResponse(session models.UserSession, currentSessionID string) ActiveSessionResponse {
	return ActiveSessionResponse{
		ID:         session.ID,
		SessionID:  session.SessionID,
		Device:     displayDevice(session.UserAgent),
		Browser:    displayBrowser(session.UserAgent),
		IPAddress:  displayOrPlaceholder(session.IPAddress),
		CreatedAt:  formatDateTime(session.CreatedAt),
		LastSeenAt: formatDateTime(session.LastSeenAt),
		Current:    session.SessionID == strings.TrimSpace(currentSessionID),
	}
}

func NewActiveSessionResponseList(sessions []models.UserSession, currentSessionID string) []ActiveSessionResponse {
	res := make([]ActiveSessionResponse, 0, len(sessions))
	for _, session := range sessions {
		res = append(res, NewActiveSessionResponse(session, currentSessionID))
	}
	return res
}

func formatDateTime(value time.Time) string {
	if value.IsZero() {
		return "Não informado"
	}
	return value.In(time.Local).Format("02/01/2006 15:04")
}

func displayBrowser(userAgent string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "edg/"):
		return "Microsoft Edge"
	case strings.Contains(ua, "opr/"), strings.Contains(ua, "opera/"):
		return "Opera"
	case strings.Contains(ua, "firefox/"):
		return "Firefox"
	case strings.Contains(ua, "chrome/"), strings.Contains(ua, "chromium/"):
		return "Chrome"
	case strings.Contains(ua, "safari/"):
		return "Safari"
	default:
		return "Navegador desconhecido"
	}
}

func displayDevice(userAgent string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "windows"):
		return "Windows"
	case strings.Contains(ua, "android"):
		return "Android"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"):
		return "iOS"
	case strings.Contains(ua, "mac os"), strings.Contains(ua, "macintosh"):
		return "macOS"
	case strings.Contains(ua, "linux"):
		return "Linux"
	default:
		return "dispositivo desconhecido"
	}
}
