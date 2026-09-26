package dto

import "strings"

func PasswordsMatch(first, second string) (password string, ok bool) {
	if first != second {
		return "", false
	}
	return first, true
}

func NewUserRequest(email, password string) (req UserRequest) {
	req.Email = strings.TrimSpace(email)
	req.Password = password
	return
}
