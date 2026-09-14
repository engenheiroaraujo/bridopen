package dto

import (
	"strings"

	models "github.com/engenheiroaraujo/bridopen/internal/handlers/users/model"
)

func PersonalFieldsFromModel(firstName, lastName, phone string) (fullName, phoneDisplay string, hasInfo bool) {
	fullName = formatFullName(firstName, lastName)
	phoneDisplay = displayOrPlaceholder(phone)
	hasInfo = strings.TrimSpace(firstName) != "" || strings.TrimSpace(lastName) != "" || strings.TrimSpace(phone) != ""
	return fullName, phoneDisplay, hasInfo
}

func formatFullName(firstName, lastName string) string {
	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	switch {
	case firstName != "" && lastName != "":
		return firstName + " " + lastName
	case firstName != "":
		return firstName
	case lastName != "":
		return lastName
	default:
		return "Não informado"
	}
}

// DisplayOrPlaceholder formata valor vazio para exibição na UI.
func DisplayOrPlaceholder(value string) string {
	return displayOrPlaceholder(value)
}

// FormatFullNameForDisplay monta nome completo para exibição.
func FormatFullNameForDisplay(firstName, lastName string) string {
	return formatFullName(firstName, lastName)
}

func displayOrPlaceholder(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Não informado"
	}
	return value
}

func TwoFactorMethodLabel(method string) string {
	return models.TwoFactorLoginMethodLabel(strings.TrimSpace(method))
}
