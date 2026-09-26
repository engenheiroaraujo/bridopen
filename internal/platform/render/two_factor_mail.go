package render

import (
	"bytes"
	"html/template"

	"github.com/engenheiroaraujo/bridopen/views"
)

type TwoFactorEmailRenderer struct{}

func NewTwoFactorEmailRenderer() TwoFactorEmailRenderer {
	return TwoFactorEmailRenderer{}
}

func (TwoFactorEmailRenderer) RenderTwoFactorCode(code string) ([]byte, error) {
	t, err := template.ParseFS(views.Files, "templates/mails/twofactor.html")
	if err != nil {
		return nil, err
	}

	var body bytes.Buffer
	if err := t.Execute(&body, map[string]string{"code": code}); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}
