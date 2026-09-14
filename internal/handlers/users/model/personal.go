package model

import "github.com/jackc/pgx/v5/pgtype"

type UserPersonalInformation struct {
	Id          pgtype.Numeric
	UserId      pgtype.Numeric
	FirstName   pgtype.Text
	LastName    pgtype.Text
	PhoneNumber pgtype.Text
	AddressId   pgtype.Numeric
	CreatedAt   pgtype.Timestamp
	UpdatedAt   pgtype.Timestamp
}
