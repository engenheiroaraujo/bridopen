package mailer

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/gomail.v2"
)

type fakeDialer struct {
	messages []*gomail.Message
	err      error
}

func (f *fakeDialer) DialAndSend(m ...*gomail.Message) error {
	f.messages = append(f.messages, m...)
	return f.err
}

// TestConsoleMailServiceSend verifica que o mailer de console imprime remetente, destinatários, assunto e corpo.
func TestConsoleMailServiceSend(t *testing.T) {
	svc := NewConsoleMailService("from@bridopen.com")

	r, w, err := os.Pipe()
	require.NoError(t, err)
	original := os.Stdout
	os.Stdout = w
	err = svc.Send(MailMessage{
		To:      []string{"a@example.com", "b@example.com"},
		Subject: "Olá",
		Body:    []byte("corpo"),
	})
	require.NoError(t, err)
	require.NoError(t, w.Close())
	os.Stdout = original

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "from@bridopen.com")
	assert.Contains(t, out, "a@example.com,b@example.com")
	assert.Contains(t, out, "Olá")
	assert.Contains(t, out, "corpo")
}

// TestNewSMTPMailServiceUsesRealDialerFactory verifica que NewSMTPMailService cria um serviço não nulo com a factory real.
func TestNewSMTPMailServiceUsesRealDialerFactory(t *testing.T) {
	t.Parallel()
	svc := NewSMTPMailService(SMTPConfig{
		Host: "localhost", Port: 2525, Username: "u", Password: "p", From: "from@bridopen.com",
	})
	require.NotNil(t, svc)
}

// TestSMTPMailServiceSendPlainAndHTML verifica envio SMTP texto/HTML e propagação de erro do dialer.
func TestSMTPMailServiceSendPlainAndHTML(t *testing.T) {
	original := newSMTPDialer
	t.Cleanup(func() { newSMTPDialer = original })

	dialer := &fakeDialer{}
	newSMTPDialer = func(string, int, string, string) mailDialer { return dialer }

	svc := NewSMTPMailService(SMTPConfig{
		Host: "localhost", Port: 2525, Username: "u", Password: "p", From: "from@bridopen.com",
	})

	require.NoError(t, svc.Send(MailMessage{
		To: []string{"to@example.com"}, Subject: "plain", Body: []byte("hello"),
	}))
	require.NoError(t, svc.Send(MailMessage{
		To: []string{"to@example.com"}, Subject: "html", Body: []byte("<p>Olá</p>"), IsHtml: true,
	}))
	require.Len(t, dialer.messages, 2)

	dialer.err = errors.New("smtp down")
	require.Error(t, svc.Send(MailMessage{To: []string{"to@example.com"}, Subject: "x", Body: []byte("y")}))
}

// TestHTMLToPlainText verifica conversão de HTML para texto plano sem tags e com entidades decodificadas.
func TestHTMLToPlainText(t *testing.T) {
	t.Parallel()

	body := `<html><body><h1>Confirmação</h1><p>Olá, <strong>Gabriel</strong> &amp; equipe.</p><p><a href="https://bridopen.com">Confirmar cadastro</a></p></body></html>`
	plain := htmlToPlainText(body)

	assert.NotContains(t, plain, "<html>")
	assert.NotContains(t, plain, "<strong>")
	assert.NotContains(t, plain, "&amp;")
	assert.Contains(t, plain, "Confirmação")
	assert.Contains(t, plain, "Olá, Gabriel & equipe.")
	assert.Contains(t, plain, "Confirmar cadastro")

	plain = htmlToPlainText("<div>Linha 1</div><br><p>Linha 2</p>\n\n\n<p></p>")
	assert.Contains(t, plain, "Linha 1")
	assert.Contains(t, plain, "Linha 2")
}
