package errors

import (
	stderrors "errors"
	"log/slog"
	"net/http"
)

/*
Toda variável de erro do projeto vive aqui, em errors.go — sem exceção, mesmo quando hoje só um pacote a usa. Antes de declarar `var Err... = errors.New(...)`
num pacote de módulo ou de platform, declare aqui e importe de lá (apperrors.ErrX). Erro construído inline dentro de uma função, sem nome, não é
variável e não entra nesta regra.
*/

// Sentinelas de domínio e persistência: erro simples, sem status HTTP embutido. Quem devolve decide como mapear para a resposta (ver errors.Is nos
// handlers/services que as usam).
var (
	ErrNotFound                           = stderrors.New("registro não encontrado")
	ErrDuplicate                          = stderrors.New("registro duplicado")
	ErrUserInactive                       = stderrors.New("utilizador sem conta ativa")
	ErrInvalidTokenOrUserAlreadyConfirmed = stderrors.New("token de redefinição de senha é inválido ou está expirado.")
	ErrInvalidCredentials                 = stderrors.New("usuário ou senha inválidos")
)

// Sentinelas com status HTTP: já chegam ao middleware de erro com o código certo.
var (
	ErrPageNotFound = WithStatus(stderrors.New("página não encontrada"), http.StatusNotFound)
	ErrInternal     = WithStatus(stderrors.New("erro interno"), http.StatusInternalServerError)
)

// Sentinelas de anexos de nota.
var (
	ErrAttachmentLimit          = stderrors.New("limite de anexos atingido")
	ErrAttachmentEmpty          = stderrors.New("arquivo vazio")
	ErrAttachmentTooLarge       = stderrors.New("arquivo muito grande")
	ErrAttachmentTypeNotAllowed = stderrors.New("tipo de arquivo nao permitido")
	ErrAttachmentInvalidName    = stderrors.New("nome de arquivo invalido")
	ErrAttachmentInvalidPath    = stderrors.New("caminho de arquivo invalido")
	ErrAttachmentMalicious      = stderrors.New("arquivo malicioso detectado")
	ErrAttachmentScanFailed     = stderrors.New("falha ao verificar arquivo")
	ErrAttachmentSaveFailed     = stderrors.New("falha ao gravar anexo")
	ErrNoteInTrash              = stderrors.New("nota na lixeira")
)

// Sentinelas de verificação em duas etapas.
var (
	ErrTwoFactorInvalidMethod     = stderrors.New("método de verificação em duas etapas inválido")
	ErrTwoFactorAlreadyEnabled    = stderrors.New("verificação em duas etapas já está ativa")
	ErrTwoFactorDisabled          = stderrors.New("verificação em duas etapas desativada")
	ErrTwoFactorInvalidCode       = stderrors.New("código de verificação inválido")
	ErrTwoFactorChallengeExpired  = stderrors.New("código de verificação expirado")
	ErrTwoFactorTooManyAttempts   = stderrors.New("limite de tentativas de verificação excedido")
	ErrTwoFactorRequiresConfirmed = stderrors.New("somente usuários confirmados podem configurar verificação em duas etapas")
	ErrTwoFactorCooldown          = stderrors.New("aguarde antes de solicitar novo código de verificação")
	ErrTwoFactorMailRenderer      = stderrors.New("renderizador de e-mail 2FA não configurado")
	ErrRecoveryCodesUnavailable   = stderrors.New("repositório de códigos de recuperação não configurado")
)

// StatusError carrega o status HTTP junto do erro.
type StatusError struct {
	error
	status int
}

func (se StatusError) StatusCode() int {
	return se.status
}

func WithStatus(err error, status int) error {
	return StatusError{
		error:  err,
		status: status,
	}
}

// RepositoryError envolve falha de banco/infra. O detalhe fica no log; a
// resposta HTTP é 500 genérica.
type RepositoryError struct {
	error
}

// NewRepositoryError registra o erro real e o devolve embrulhado. É o único caminho para erro vindo do banco: o detalhe fica no log e a resposta HTTP é
// 500 genérica, sem expor informação técnica ao utilizador. Devolve nil para err nil, para o chamador poder repassar o resultado sem checagem extra.
func NewRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	slog.Error(err.Error())
	return &RepositoryError{error: err}
}

func (e *RepositoryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.error
}

/*
Mensagens de erro voltadas ao utilizador.

Ficam aqui, junto das sentinelas que traduzem: a sentinela e o texto que o utilizador lê são a mesma decisão, e mantê-los em pacotes diferentes é como as
duas listas saem de sincronia. Ao acrescentar uma sentinela com mensagem própria, acrescente o case correspondente na função abaixo.
*/

// AttachmentUploadMessage traduz a falha de upload de anexo na mensagem exibida no formulário.
func AttachmentUploadMessage(err error) string {
	switch {
	case stderrors.Is(err, ErrAttachmentTooLarge):
		return "Arquivo muito grande. Use arquivos de até 10 MB."
	case stderrors.Is(err, ErrAttachmentTypeNotAllowed):
		return "Tipo de arquivo não permitido. Use imagens, PDF, TXT ou DOCX."
	case stderrors.Is(err, ErrAttachmentEmpty):
		return "O arquivo selecionado está vazio."
	case stderrors.Is(err, ErrAttachmentInvalidName):
		return "Nome de arquivo inválido."
	case stderrors.Is(err, ErrAttachmentMalicious):
		return "Arquivo bloqueado pela verificação de segurança."
	case stderrors.Is(err, ErrAttachmentScanFailed):
		return "Não foi possível verificar o arquivo com segurança. Tente novamente."
	default:
		return "Não foi possível anexar o arquivo. Tente novamente."
	}
}

// TwoFactorMessage traduz a falha de verificação em duas etapas na mensagem exibida no formulário.
func TwoFactorMessage(err error) string {
	switch {
	case stderrors.Is(err, ErrRecoveryCodesUnavailable):
		return "Não foi possível gerar códigos de recuperação agora."
	case stderrors.Is(err, ErrTwoFactorInvalidCode):
		return "Código de verificação inválido."
	case stderrors.Is(err, ErrTwoFactorChallengeExpired):
		return "Código expirado. Faça login novamente para receber um novo código."
	case stderrors.Is(err, ErrTwoFactorTooManyAttempts):
		return "Limite de tentativas excedido. Faça login novamente."
	case stderrors.Is(err, ErrTwoFactorCooldown):
		return "Aguarde um pouco antes de solicitar outro código."
	case stderrors.Is(err, ErrTwoFactorRequiresConfirmed):
		return "A conta precisa estar confirmada para usar verificação em duas etapas."
	case stderrors.Is(err, ErrTwoFactorInvalidMethod):
		return "Método de verificação em duas etapas inválido."
	case stderrors.Is(err, ErrTwoFactorAlreadyEnabled):
		return "A verificação em duas etapas já está ativa."
	case stderrors.Is(err, ErrTwoFactorDisabled):
		return "A verificação em duas etapas já está desativada."
	default:
		return "Não foi possível validar o código agora."
	}
}
