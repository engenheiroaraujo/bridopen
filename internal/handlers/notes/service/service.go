package service

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	notedto "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/dto"
	models "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/model"
	noterepo "github.com/engenheiroaraujo/bridopen/internal/handlers/notes/repositories"
	"github.com/engenheiroaraujo/bridopen/internal/platform/audit"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"github.com/engenheiroaraujo/bridopen/internal/platform/validations"
)

// NoteRepository é o contrato que a camada de serviço precisa do armazenamento.
// Fica declarado aqui, no consumidor: o pacote de repositório não conhece esta
// interface, apenas satisfaz os métodos.
type NoteRepository interface {
	GetById(ctx context.Context, userId, id int) (*models.Note, error)
	Create(ctx context.Context, userId int, title, content, color string, tagIDs []int) (*models.Note, error)
	Update(ctx context.Context, userId, id int, title, content, color string, tagIDs []int) (*models.Note, error)
	Delete(ctx context.Context, userId, id int) error

	List(ctx context.Context, userId int, filter models.NoteFilter) ([]models.Note, error)
	ListColors(ctx context.Context, userId int) ([]string, error)
	ListArchived(ctx context.Context, userId int) ([]models.Note, error)
	ListDeleted(ctx context.Context, userId int) ([]models.Note, error)

	ListTags(ctx context.Context, userId int) ([]models.NoteTag, error)
	CreateTag(ctx context.Context, userId int, name, color string) (*models.NoteTag, error)
	UpdateTag(ctx context.Context, userId, id int, name, color string) (*models.NoteTag, error)
	DeleteTag(ctx context.Context, userId, id int) error

	SetPinned(ctx context.Context, userId int, id int, pinned bool) error
	Archive(ctx context.Context, userId int, id int) error
	Unarchive(ctx context.Context, userId int, id int) error
	MoveToTrash(ctx context.Context, userId int, id int) error
	Restore(ctx context.Context, userId int, id int) error
	DeletePermanently(ctx context.Context, userId int, id int) error

	ListAttachments(ctx context.Context, userId, noteId int) ([]models.NoteAttachment, error)
	GetAttachment(ctx context.Context, userId, noteId, attachmentId int) (*models.NoteAttachment, error)
	CreateAttachment(ctx context.Context, userId, noteId int, input noterepo.NoteAttachmentInput) (*models.NoteAttachment, error)
	DeleteAttachment(ctx context.Context, userId, noteId, attachmentId int) (*models.NoteAttachment, error)
}

// classificar o erro sem importar o pacote de repositório.

// arquivo, e não no banco. O handler usa essa marca para responder com a
// mensagem de formulário (422) em vez de tratar como erro interno.

type NoteService struct {
	repo    NoteRepository
	storage AttachmentStorage
	audit   audit.Recorder
}

func NewNoteService(repo NoteRepository, storage AttachmentStorage, auditRecorder audit.Recorder) *NoteService {
	return &NoteService{repo: repo, storage: storage, audit: auditRecorder}
}

// NoteListResult reúne o que a tela de listagem precisa numa única chamada.
type NoteListResult struct {
	Notes  []models.Note
	Colors []string
	Tags   []models.NoteTag
}

// SaveNoteInput são os dados de nota já decodificados da requisição.
type SaveNoteInput struct {
	Id      int
	Title   string
	Content string
	Color   string
	TagIDs  []int
}

// SaveNoteResult carrega o resultado da gravação ou os erros de validação,
// junto das etiquetas disponíveis para o handler remontar o formulário.
type SaveNoteResult struct {
	Note          *models.Note
	Created       bool
	TagIDs        []int
	AvailableTags []models.NoteTag
	FieldErrors   map[string]string
}

// -----------------------------------------------------------------------------
// Listagens
// -----------------------------------------------------------------------------

func (s *NoteService) List(ctx context.Context, userID int, filter models.NoteFilter) (NoteListResult, error) {
	var result NoteListResult

	notes, err := s.repo.List(ctx, userID, filter)
	if err != nil {
		return result, err
	}
	colors, err := s.repo.ListColors(ctx, userID)
	if err != nil {
		return result, err
	}
	tags, err := s.repo.ListTags(ctx, userID)
	if err != nil {
		return result, err
	}

	result.Notes = notes
	result.Colors = colors
	result.Tags = tags
	return result, nil
}

func (s *NoteService) Get(ctx context.Context, userID, id int) (*models.Note, error) {
	return s.repo.GetById(ctx, userID, id)
}

func (s *NoteService) ListTags(ctx context.Context, userID int) ([]models.NoteTag, error) {
	return s.repo.ListTags(ctx, userID)
}

func (s *NoteService) ListArchived(ctx context.Context, userID int) ([]models.Note, error) {
	return s.repo.ListArchived(ctx, userID)
}

func (s *NoteService) ListDeleted(ctx context.Context, userID int) ([]models.Note, error) {
	return s.repo.ListDeleted(ctx, userID)
}

// -----------------------------------------------------------------------------
// Gravação de nota
// -----------------------------------------------------------------------------

// Save valida a nota, decide entre criar e atualizar e registra a auditoria.
// Quando há erro de validação devolve FieldErrors preenchido e Note nulo, sem
// tocar no repositório.
func (s *NoteService) Save(ctx context.Context, userID int, input SaveNoteInput) (SaveNoteResult, error) {
	result := SaveNoteResult{FieldErrors: map[string]string{}}

	availableTags, err := s.repo.ListTags(ctx, userID)
	if err != nil {
		return result, err
	}
	result.AvailableTags = availableTags

	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.Content)

	tagIDs, tagErr := notedto.ValidateNoteTagSelection(input.TagIDs, availableTags)
	result.TagIDs = tagIDs

	if title == "" {
		result.FieldErrors["title"] = "Título é obrigatório."
	}
	if len([]rune(title)) > validations.MaxNoteTitleLen {
		result.FieldErrors["title"] = "Título deve conter no máximo 50 caracteres."
	}
	if content == "" {
		result.FieldErrors["content"] = "Conteúdo é obrigatório."
	}
	if tagErr != "" {
		result.FieldErrors["tags"] = tagErr
	}
	if len(result.FieldErrors) > 0 {
		return result, nil
	}

	var note *models.Note
	if input.Id > 0 {
		note, err = s.repo.Update(ctx, userID, input.Id, title, content, input.Color, tagIDs)
	} else {
		result.Created = true
		note, err = s.repo.Create(ctx, userID, title, content, input.Color, tagIDs)
	}
	if err != nil {
		return result, err
	}
	result.Note = note

	noteID := ""
	if note.Id.Int != nil {
		noteID = note.Id.Int.String()
	}
	if result.Created {
		slog.Info("nota criada", slog.String("id", noteID), slog.Int64("user_id", int64(userID)))
		if note.Id.Int != nil {
			s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataCreated, audit.EntityNote, note.Id.Int.Int64())
		}
	} else {
		slog.Info("nota atualizada", slog.String("id", noteID), slog.Int64("user_id", int64(userID)))
		s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataUpdated, audit.EntityNote, int64(input.Id))
	}

	return result, nil
}

// -----------------------------------------------------------------------------
// Estado da nota
// -----------------------------------------------------------------------------

func (s *NoteService) MoveToTrash(ctx context.Context, userID, id int) error {
	if err := s.repo.Delete(ctx, userID, id); err != nil {
		return err
	}
	slog.Info("nota movida para lixeira", slog.Int("id", id), slog.Int64("user_id", int64(userID)))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataDeleted, audit.EntityNote, int64(id))
	return nil
}

func (s *NoteService) SetPinned(ctx context.Context, userID, id int, pinned bool) error {
	if err := s.repo.SetPinned(ctx, userID, id, pinned); err != nil {
		return err
	}
	message := "nota fixada"
	if !pinned {
		message = "nota desafixada"
	}
	slog.Info(message, slog.Int("id", id), slog.Int64("user_id", int64(userID)))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataUpdated, audit.EntityNote, int64(id))
	return nil
}

func (s *NoteService) Archive(ctx context.Context, userID, id int) error {
	if err := s.repo.Archive(ctx, userID, id); err != nil {
		return err
	}
	slog.Info("nota arquivada", slog.Int("id", id), slog.Int64("user_id", int64(userID)))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataUpdated, audit.EntityNote, int64(id))
	return nil
}

func (s *NoteService) Unarchive(ctx context.Context, userID, id int) error {
	if err := s.repo.Unarchive(ctx, userID, id); err != nil {
		return err
	}
	slog.Info("nota desarquivada", slog.Int("id", id), slog.Int64("user_id", int64(userID)))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataUpdated, audit.EntityNote, int64(id))
	return nil
}

func (s *NoteService) Restore(ctx context.Context, userID, id int) error {
	if err := s.repo.Restore(ctx, userID, id); err != nil {
		return err
	}
	slog.Info("nota restaurada", slog.Int("id", id), slog.Int64("user_id", int64(userID)))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataRestored, audit.EntityNote, int64(id))
	return nil
}

// DeletePermanently apaga a nota e, em seguida, os arquivos físicos dos anexos.
// Falha ao remover arquivo é apenas registrada: a nota já saiu do banco.
func (s *NoteService) DeletePermanently(ctx context.Context, userID, id int) error {
	attachments, err := s.repo.ListAttachments(ctx, userID, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeletePermanently(ctx, userID, id); err != nil {
		return err
	}
	for _, attachment := range attachments {
		if attachment.StorageKey.Valid {
			if err := s.storage.Delete(ctx, attachment.StorageKey.String); err != nil {
				slog.Warn("falha ao remover arquivo fisico do anexo", slog.String("err", err.Error()), slog.String("storage_key", attachment.StorageKey.String))
			}
		}
	}
	slog.Info("nota excluida definitivamente", slog.Int("id", id), slog.Int64("user_id", int64(userID)))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataDeleted, audit.EntityNote, int64(id))
	return nil
}

// -----------------------------------------------------------------------------
// Etiquetas
// -----------------------------------------------------------------------------

func (s *NoteService) CreateTag(ctx context.Context, userID int, name, color string) (*models.NoteTag, error) {
	tag, err := s.repo.CreateTag(ctx, userID, name, color)
	if err != nil {
		return nil, err
	}
	tagID := int64(0)
	if tag.Id.Int != nil {
		tagID = tag.Id.Int.Int64()
	}
	slog.Info("etiqueta criada", slog.Int64("id", tagID), slog.Int64("user_id", int64(userID)))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataCreated, audit.EntityNoteTag, tagID)
	return tag, nil
}

func (s *NoteService) UpdateTag(ctx context.Context, userID, id int, name, color string) (*models.NoteTag, error) {
	tag, err := s.repo.UpdateTag(ctx, userID, id, name, color)
	if err != nil {
		return nil, err
	}
	tagID := int64(id)
	if tag.Id.Int != nil {
		tagID = tag.Id.Int.Int64()
	}
	slog.Info("etiqueta atualizada", slog.Int("id", id), slog.Int64("user_id", int64(userID)))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataUpdated, audit.EntityNoteTag, tagID)
	return tag, nil
}

func (s *NoteService) DeleteTag(ctx context.Context, userID, id int) error {
	if err := s.repo.DeleteTag(ctx, userID, id); err != nil {
		return err
	}
	slog.Info("etiqueta excluida", slog.Int("id", id), slog.Int64("user_id", int64(userID)))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventDataDeleted, audit.EntityNoteTag, int64(id))
	return nil
}

// RecordTagValidationFailure registra a auditoria de formulário de etiqueta
// inválido, que é decidida no handler mas é evento de domínio.
func (s *NoteService) RecordTagValidationFailure(ctx context.Context, userID int64, tagID int64) {
	s.audit.RecordFailure(ctx, userID, audit.EventErrorValidation, audit.EntityNoteTag, tagID)
}

// -----------------------------------------------------------------------------
// Anexos
// -----------------------------------------------------------------------------

// UploadAttachment grava o arquivo, registra os metadados e desfaz a gravação
// física se o banco recusar o registro.
func (s *NoteService) UploadAttachment(ctx context.Context, userID, noteID int, file multipart.File, header *multipart.FileHeader) (*models.NoteAttachment, error) {
	note, err := s.repo.GetById(ctx, userID, noteID)
	if err != nil {
		return nil, err
	}
	if note.DeletedAt.Valid {
		return nil, apperrors.ErrNoteInTrash
	}

	stored, err := s.storage.Save(ctx, userID, noteID, file, header)
	if err != nil {
		s.audit.RecordFailure(ctx, int64(userID), audit.EventFileInvalid, audit.EntityNoteAttachment, int64(noteID))
		// errors.Join preserva o sentinel original (apperrors.ErrAttachmentTooLarge etc.)
		// para errors.Is, acrescentando a marca de fase.
		return nil, errors.Join(apperrors.ErrAttachmentSaveFailed, err)
	}

	attachment, err := s.repo.CreateAttachment(ctx, userID, noteID, noterepo.NoteAttachmentInput{
		OriginalName:   stored.OriginalName,
		StorageKey:     stored.StorageKey,
		MimeType:       stored.MimeType,
		SizeBytes:      stored.SizeBytes,
		ChecksumSHA256: stored.ChecksumSHA256,
	})
	if err != nil {
		if deleteErr := s.storage.Delete(ctx, stored.StorageKey); deleteErr != nil {
			slog.Warn("falha ao remover arquivo apos erro ao salvar metadados", slog.String("err", deleteErr.Error()), slog.String("storage_key", stored.StorageKey))
		}
		if errors.Is(err, apperrors.ErrAttachmentLimit) {
			s.audit.RecordFailure(ctx, int64(userID), audit.EventErrorLimitExceeded, audit.EntityNoteAttachment, int64(noteID))
		}
		return nil, err
	}

	attachmentID := int64(0)
	if attachment.Id.Int != nil {
		attachmentID = attachment.Id.Int.Int64()
	}
	slog.Info("anexo enviado", slog.Int("note_id", noteID), slog.Int64("attachment_id", attachmentID), slog.Int("user_id", userID))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventFileUploaded, audit.EntityNoteAttachment, attachmentID)
	return attachment, nil
}

// GetAttachment devolve os metadados do anexo junto do caminho físico do arquivo.
func (s *NoteService) GetAttachment(ctx context.Context, userID, noteID, attachmentID int) (*models.NoteAttachment, string, error) {
	attachment, err := s.repo.GetAttachment(ctx, userID, noteID, attachmentID)
	if err != nil {
		return nil, "", err
	}
	filePath, err := s.storage.Path(attachment.StorageKey.String)
	if err != nil {
		return attachment, "", err
	}
	return attachment, filePath, nil
}

// DeleteAttachment remove o registro e o arquivo físico correspondente.
func (s *NoteService) DeleteAttachment(ctx context.Context, userID, noteID, attachmentID int) error {
	attachment, err := s.repo.DeleteAttachment(ctx, userID, noteID, attachmentID)
	if err != nil {
		return err
	}
	if attachment.StorageKey.Valid {
		if err := s.storage.Delete(ctx, attachment.StorageKey.String); err != nil {
			slog.Warn("falha ao remover arquivo fisico do anexo", slog.String("err", err.Error()), slog.String("storage_key", attachment.StorageKey.String))
		}
	}
	slog.Info("anexo removido", slog.Int("note_id", noteID), slog.Int("attachment_id", attachmentID), slog.Int("user_id", userID))
	s.audit.RecordSuccess(ctx, int64(userID), audit.EventFileDeleted, audit.EntityNoteAttachment, int64(attachmentID))
	return nil
}

// =============================================================================
// Armazenamento físico de anexos
// =============================================================================

const MaxAttachmentSizeBytes = 10 << 20

var (

	// cryptoRandRead é substituível nos testes para reproduzir de forma determinística
	// uma colisão de nome de arquivo (Save falhando ao renomear para um destino já ocupado).
	cryptoRandRead = rand.Read
	// osCreateTemp é substituível nos testes para simular falha real de I/O
	// (disco cheio, permissão negada) ao criar o arquivo temporário do upload.
	osCreateTemp = func(dir, pattern string) (uploadTempFile, error) {
		return os.CreateTemp(dir, pattern)
	}
)

type uploadTempFile interface {
	io.Writer
	Name() string
	Close() error
}

type StoredAttachment struct {
	OriginalName   string
	StorageKey     string
	MimeType       string
	SizeBytes      int64
	ChecksumSHA256 string
}

type AttachmentStorage interface {
	Save(ctx context.Context, userID, noteID int, file multipart.File, header *multipart.FileHeader) (StoredAttachment, error)
	Path(storageKey string) (string, error)
	Delete(ctx context.Context, storageKey string) error
}

type LocalAttachmentStorage struct {
	root    string
	scanner AttachmentScanner
}

func NewLocalAttachmentStorage(root string, scanners ...AttachmentScanner) *LocalAttachmentStorage {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "storage/attachments"
	}
	scanner := AttachmentScanner(NoopAttachmentScanner{})
	if len(scanners) > 0 && scanners[0] != nil {
		scanner = scanners[0]
	}
	return &LocalAttachmentStorage{root: root, scanner: scanner}
}

func (s *LocalAttachmentStorage) Save(ctx context.Context, userID, noteID int, file multipart.File, header *multipart.FileHeader) (StoredAttachment, error) {
	if err := ctx.Err(); err != nil {
		return StoredAttachment{}, err
	}
	if file == nil || header == nil {
		return StoredAttachment{}, apperrors.ErrAttachmentEmpty
	}
	if header.Size <= 0 {
		return StoredAttachment{}, apperrors.ErrAttachmentEmpty
	}
	if header.Size > MaxAttachmentSizeBytes {
		return StoredAttachment{}, apperrors.ErrAttachmentTooLarge
	}

	originalName := sanitizeAttachmentName(header.Filename)
	if originalName == "" {
		return StoredAttachment{}, apperrors.ErrAttachmentInvalidName
	}

	ext := strings.ToLower(filepath.Ext(originalName))
	mimeType, ok := allowedAttachmentMime(ext)
	if !ok {
		return StoredAttachment{}, apperrors.ErrAttachmentTypeNotAllowed
	}
	if err := validateDetectedMime(file, ext, header.Size); err != nil {
		return StoredAttachment{}, err
	}

	fileName, err := randomAttachmentFileName(ext)
	if err != nil {
		return StoredAttachment{}, err
	}
	storageKey := path.Join(strconv.Itoa(userID), strconv.Itoa(noteID), fileName)
	dir := filepath.Join(s.root, strconv.Itoa(userID), strconv.Itoa(noteID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return StoredAttachment{}, err
	}

	tmp, err := osCreateTemp(dir, ".upload-*")
	if err != nil {
		return StoredAttachment{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(file, MaxAttachmentSizeBytes+1))
	closeErr := tmp.Close()
	if err != nil {
		return StoredAttachment{}, err
	}
	if closeErr != nil {
		return StoredAttachment{}, closeErr
	}
	if written <= 0 {
		return StoredAttachment{}, apperrors.ErrAttachmentEmpty
	}
	if written > MaxAttachmentSizeBytes {
		return StoredAttachment{}, apperrors.ErrAttachmentTooLarge
	}
	if shouldScanAttachment(ext) {
		if err := s.scanner.ScanFile(ctx, tmpPath); err != nil {
			return StoredAttachment{}, err
		}
	}

	finalPath := filepath.Join(dir, fileName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return StoredAttachment{}, err
	}

	return StoredAttachment{
		OriginalName:   originalName,
		StorageKey:     storageKey,
		MimeType:       mimeType,
		SizeBytes:      written,
		ChecksumSHA256: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func (s *LocalAttachmentStorage) Path(storageKey string) (string, error) {
	storageKey = strings.TrimSpace(storageKey)
	if storageKey == "" {
		return "", apperrors.ErrAttachmentInvalidPath
	}
	clean := path.Clean("/" + storageKey)
	if clean == "/" || strings.Contains(clean, "..") {
		return "", apperrors.ErrAttachmentInvalidPath
	}
	rel := strings.TrimPrefix(clean, "/")
	localPath := filepath.Join(s.root, filepath.FromSlash(rel))

	rootAbs, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	localAbs, err := filepath.Abs(localPath)
	if err != nil {
		return "", err
	}
	relToRoot, err := filepath.Rel(rootAbs, localAbs)
	if err != nil || strings.HasPrefix(relToRoot, "..") || filepath.IsAbs(relToRoot) {
		return "", apperrors.ErrAttachmentInvalidPath
	}
	return localAbs, nil
}

func (s *LocalAttachmentStorage) Delete(ctx context.Context, storageKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	filePath, err := s.Path(storageKey)
	if err != nil {
		return err
	}
	if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func sanitizeAttachmentName(name string) string {
	// path.Base (só barra normal) em vez de filepath.Base: o nome vem do cliente
	// (pode conter "\" de um Windows), e filepath.Base só trata "\" como separador
	// quando o servidor roda em GOOS=windows — em produção (Linux) ele ignoraria.
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\':
			return -1
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	if len([]rune(name)) > 160 {
		runes := []rune(name)
		ext := filepath.Ext(name)
		limit := 160 - len([]rune(ext))
		if limit < 1 {
			return ""
		}
		name = string(runes[:limit]) + ext
	}
	return name
}

func allowedAttachmentMime(ext string) (string, bool) {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".png":
		return "image/png", true
	case ".webp":
		return "image/webp", true
	case ".pdf":
		return "application/pdf", true
	case ".txt":
		return "text/plain", true
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true
	default:
		return "", false
	}
}

func shouldScanAttachment(ext string) bool {
	switch strings.ToLower(ext) {
	case ".pdf", ".docx":
		return true
	default:
		return false
	}
}

func validateDetectedMime(file multipart.File, ext string, size int64) error {
	var sample [512]byte
	n, err := file.Read(sample[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if n == 0 {
		return apperrors.ErrAttachmentEmpty
	}
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(sample[:n]), ";")[0]))
	switch ext {
	case ".jpg", ".jpeg":
		if detected == "image/jpeg" {
			return nil
		}
	case ".png":
		if detected == "image/png" {
			return nil
		}
	case ".webp":
		if detected == "image/webp" {
			return nil
		}
	case ".pdf":
		if detected == "application/pdf" {
			return nil
		}
	case ".txt":
		if detected == "text/plain" {
			return nil
		}
	case ".docx":
		if (detected == "application/zip" || detected == "application/vnd.openxmlformats-officedocument.wordprocessingml.document") && isDocxPackage(file, size) {
			return nil
		}
	}
	return apperrors.ErrAttachmentTypeNotAllowed
}

func isDocxPackage(file multipart.File, size int64) bool {
	reader, err := zip.NewReader(file, size)
	if err != nil {
		return false
	}
	hasContentTypes := false
	hasDocument := false
	for _, f := range reader.File {
		switch f.Name {
		case "[Content_Types].xml":
			hasContentTypes = true
		case "word/document.xml":
			hasDocument = true
		}
		if hasContentTypes && hasDocument {
			return true
		}
	}
	return false
}

func randomAttachmentFileName(ext string) (string, error) {
	buf := make([]byte, 16)
	if _, err := cryptoRandRead(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s", hex.EncodeToString(buf), ext), nil
}

// =============================================================================
// Verificação antivírus de anexos
// =============================================================================

var (

	// clamDialContext é substituível nos testes para caminhos de falha de escrita na conexão.
	clamDialContext = func(ctx context.Context, timeout time.Duration, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: timeout}
		return d.DialContext(ctx, network, address)
	}
)

type AttachmentScanner interface {
	ScanFile(ctx context.Context, filePath string) error
}

type NoopAttachmentScanner struct{}

func (NoopAttachmentScanner) ScanFile(context.Context, string) error {
	return nil
}

type ClamAVScanner struct {
	addr    string
	timeout time.Duration
}

func NewClamAVScanner(addr string) *ClamAVScanner {
	return &ClamAVScanner{
		addr:    strings.TrimSpace(addr),
		timeout: 30 * time.Second,
	}
}

func (s *ClamAVScanner) ScanFile(ctx context.Context, filePath string) error {
	if s == nil || s.addr == "" {
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	conn, err := clamDialContext(ctx, s.timeout, "tcp", s.addr)
	if err != nil {
		return apperrors.ErrAttachmentScanFailed
	}
	defer conn.Close()

	deadline := time.Now().Add(s.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)

	if _, err := io.WriteString(conn, "nINSTREAM\n"); err != nil {
		return apperrors.ErrAttachmentScanFailed
	}

	var sizeBuf [4]byte
	buf := make([]byte, 32*1024)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			binary.BigEndian.PutUint32(sizeBuf[:], uint32(n))
			if _, err := conn.Write(sizeBuf[:]); err != nil {
				return apperrors.ErrAttachmentScanFailed
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return apperrors.ErrAttachmentScanFailed
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	binary.BigEndian.PutUint32(sizeBuf[:], 0)
	if _, err := conn.Write(sizeBuf[:]); err != nil {
		return apperrors.ErrAttachmentScanFailed
	}

	response, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return apperrors.ErrAttachmentScanFailed
	}
	response = strings.TrimSpace(response)
	switch {
	case strings.Contains(response, "FOUND"):
		return apperrors.ErrAttachmentMalicious
	case strings.Contains(response, "OK"):
		return nil
	default:
		return apperrors.ErrAttachmentScanFailed
	}
}
