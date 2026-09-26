package password

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type PasswordUtilis interface {
	HashPassword(password string) (string, error)
}

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("Falha ao gerar a hash da senha")
	}
	return string(hashed), nil
}

func ValidatePassword(password, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}