package dto

import (
	"html/template"

	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

type UserPersonalResponse struct {
	Id          int
	UserId      int64
	FirstName   string
	LastName    string
	PhoneNumber string
}

type UserPersonalRequest struct {
	Id          int
	UserId      int64
	FirstName   string
	LastName    string
	PhoneNumber string
	CSRFField   template.HTML
	validations.FormValidator
}

func NewUserPersonalRequest(id int, userId int64, firstname, lastname, phonenumber string) UserPersonalRequest {
	return UserPersonalRequest{
		Id:          id,
		UserId:      userId,
		FirstName:   firstname,
		LastName:    lastname,
		PhoneNumber: phonenumber,
	}
}
