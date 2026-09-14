package dto

import (
	"strings"
	"time"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
)

func NewMePage(email, recoveryEmail, pendingRecoveryEmail, firstName, lastName, phone string, createdAt *time.Time) MePage {
	page := MePage{
		Email:                email,
		RecoveryEmail:        displayOrPlaceholder(recoveryEmail),
		PendingRecoveryEmail: strings.TrimSpace(pendingRecoveryEmail),
		FirstName:            strings.TrimSpace(firstName),
		LastName:             strings.TrimSpace(lastName),
		Phone:                strings.TrimSpace(phone),
		PhoneNumber:          displayOrPlaceholder(phone),
		NavPath:              "/me",
	}
	page.FullName = formatFullName(firstName, lastName)
	page.HasPersonalInfo = strings.TrimSpace(firstName) != "" || strings.TrimSpace(lastName) != "" || strings.TrimSpace(phone) != ""
	page.MemberSince = formatMemberSince(createdAt)
	page.SetTwoFactorStatus(true, false, "")
	return page
}

func (p *MePage) SetTwoFactorStatus(active, enabled bool, method string) {
	p.SetTwoFactorStatusView(models.NewTwoFactorStatusViewFromFields(active, enabled, strings.TrimSpace(method)))
}

func (p *MePage) SetTwoFactorStatusView(status *models.TwoFactorStatusView) {
	if status == nil {
		status = models.NewTwoFactorStatusView(nil)
	}
	p.TwoFactorActive = status.Active
	p.TwoFactorEnabled = status.Enabled
	p.TwoFactorMethod = strings.TrimSpace(status.Method)
	p.TwoFactorStatus = status.StatusLabel
	p.TwoFactorActionLabel = status.ActionLabel
}

func (p *MePage) SetSelectedTwoFactorMethod(method, title, description string) {
	p.SelectedTwoFactorMethod = strings.TrimSpace(method)
	p.SelectedTwoFactorMethodTitle = strings.TrimSpace(title)
	p.SelectedTwoFactorMethodDescription = strings.TrimSpace(description)
}

func formatMemberSince(createdAt *time.Time) string {
	if createdAt == nil {
		return "Não informado"
	}
	return createdAt.In(time.Local).Format("02/01/2006")
}
