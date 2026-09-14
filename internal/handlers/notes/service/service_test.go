package service

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	apperrors "github.com/engenheiroaraujo/bridopen/internal/platform/errors"
	"io"
	"mime/multipart"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewClamAVScanner verifica a criação do scanner ClamAV com endereço normalizado e timeout padrão.
func TestNewClamAVScanner(t *testing.T) {

	s := NewClamAVScanner(" 127.0.0.1:3310 ")
	require.NotNil(t, s)
	assert.Equal(t, "127.0.0.1:3310", s.addr)
	assert.Equal(t, 30*time.Second, s.timeout)
}

// TestClamAVScannerNilOrEmptyAddr verifica que scanner nulo ou sem endereço não falha ao escanear.
func TestClamAVScannerNilOrEmptyAddr(t *testing.T) {

	var nilScanner *ClamAVScanner
	assert.NoError(t, nilScanner.ScanFile(context.Background(), "any"))
	assert.NoError(t, NewClamAVScanner("").ScanFile(context.Background(), "any"))
	assert.NoError(t, NewClamAVScanner("   ").ScanFile(context.Background(), "any"))
}

// TestClamAVScannerOpenFileError verifica erro ao abrir arquivo inexistente para varredura.
func TestClamAVScannerOpenFileError(t *testing.T) {

	s := NewClamAVScanner("127.0.0.1:1")
	err := s.ScanFile(context.Background(), filepath.Join(t.TempDir(), "missing.bin"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

// TestClamAVScannerDialFailure verifica falha de conexão com o daemon ClamAV.
func TestClamAVScannerDialFailure(t *testing.T) {

	path := filepath.Join(t.TempDir(), "file.bin")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))

	s := NewClamAVScanner("127.0.0.1:1")
	s.timeout = 200 * time.Millisecond
	assert.ErrorIs(t, s.ScanFile(context.Background(), path), apperrors.ErrAttachmentScanFailed)
}

// TestClamAVScannerOK verifica varredura bem-sucedida quando ClamAV responde OK.
func TestClamAVScannerOK(t *testing.T) {

	addr := startFakeClamAV(t, clamAVResponseOK)
	path := filepath.Join(t.TempDir(), "clean.bin")
	require.NoError(t, os.WriteFile(path, []byte("clean payload data"), 0o600))

	s := NewClamAVScanner(addr)
	s.timeout = 2 * time.Second
	assert.NoError(t, s.ScanFile(context.Background(), path))
}

// TestClamAVScannerFound verifica retorno de apperrors.ErrAttachmentMalicious quando ClamAV encontra ameaça.
func TestClamAVScannerFound(t *testing.T) {

	addr := startFakeClamAV(t, clamAVResponseFound)
	path := filepath.Join(t.TempDir(), "bad.bin")
	require.NoError(t, os.WriteFile(path, []byte("eicar-like"), 0o600))

	s := NewClamAVScanner(addr)
	s.timeout = 2 * time.Second
	assert.ErrorIs(t, s.ScanFile(context.Background(), path), apperrors.ErrAttachmentMalicious)
}

// TestClamAVScannerUnexpectedResponse verifica falha de scan para resposta inesperada do ClamAV.
func TestClamAVScannerUnexpectedResponse(t *testing.T) {

	addr := startFakeClamAV(t, clamAVResponseOther)
	path := filepath.Join(t.TempDir(), "file.bin")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))

	s := NewClamAVScanner(addr)
	s.timeout = 2 * time.Second
	assert.ErrorIs(t, s.ScanFile(context.Background(), path), apperrors.ErrAttachmentScanFailed)
}

// TestClamAVScannerWriteFailures verifica falha de scan quando a conexão fecha durante as escritas INSTREAM.
func TestClamAVScannerWriteFailures(t *testing.T) {

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		_ = conn.Close() // fecha antes do cliente terminar as escritas INSTREAM
	}()

	path := filepath.Join(t.TempDir(), "file.bin")
	require.NoError(t, os.WriteFile(path, bytesRepeat(64*1024), 0o600))

	s := NewClamAVScanner(ln.Addr().String())
	s.timeout = 2 * time.Second
	assert.ErrorIs(t, s.ScanFile(context.Background(), path), apperrors.ErrAttachmentScanFailed)
}

// TestClamAVScannerRespectsContextDeadline verifica que o deadline do contexto interrompe a varredura.
func TestClamAVScannerRespectsContextDeadline(t *testing.T) {

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, _ := reader.ReadString('\n')
		if !strings.HasPrefix(line, "nINSTREAM") {
			return
		}
		// lê o stream lentamente / trava até o deadline do cliente
		buf := make([]byte, 4)
		for {
			if _, readErr := io.ReadFull(reader, buf); readErr != nil {
				return
			}
			n := binary.BigEndian.Uint32(buf)
			if n == 0 {
				time.Sleep(2 * time.Second)
				_, _ = io.WriteString(conn, "stream: OK\n")
				return
			}
			_, _ = io.CopyN(io.Discard, reader, int64(n))
		}
	}()

	path := filepath.Join(t.TempDir(), "file.bin")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	s := NewClamAVScanner(ln.Addr().String())
	s.timeout = 5 * time.Second
	err = s.ScanFile(ctx, path)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrAttachmentScanFailed)
}

// TestClamAVScannerNoResponse verifica falha de scan quando o daemon não envia resposta.
func TestClamAVScannerNoResponse(t *testing.T) {

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = reader.ReadString('\n')
		sizeBuf := make([]byte, 4)
		for {
			if _, readErr := io.ReadFull(reader, sizeBuf); readErr != nil {
				return
			}
			n := binary.BigEndian.Uint32(sizeBuf)
			if n == 0 {
				// fecha sem escrever uma linha de resposta
				return
			}
			_, _ = io.CopyN(io.Discard, reader, int64(n))
		}
	}()

	path := filepath.Join(t.TempDir(), "file.bin")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))
	s := NewClamAVScanner(ln.Addr().String())
	s.timeout = 2 * time.Second
	// resposta vazia / EOF sem OK|FOUND → scan falhou
	assert.ErrorIs(t, s.ScanFile(context.Background(), path), apperrors.ErrAttachmentScanFailed)
}

// TestClamAVScannerReadDirectoryError verifica erro ao tentar varrer um diretório em vez de um arquivo.
func TestClamAVScannerReadDirectoryError(t *testing.T) {

	addr := startFakeClamAV(t, clamAVResponseOK)
	s := NewClamAVScanner(addr)
	s.timeout = 2 * time.Second
	err := s.ScanFile(context.Background(), t.TempDir())
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrAttachmentMalicious)
}

// TestClamAVScannerChunkWriteFailures verifica falha de scan quando a escrita de chunks INSTREAM falha.
func TestClamAVScannerChunkWriteFailures(t *testing.T) {

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		reader := bufio.NewReader(conn)
		_, _ = reader.ReadString('\n')
		_ = conn.Close() // falha nas escritas de chunks seguintes
	}()

	path := filepath.Join(t.TempDir(), "file.bin")
	require.NoError(t, os.WriteFile(path, bytesRepeat(8*1024), 0o600))
	s := NewClamAVScanner(ln.Addr().String())
	s.timeout = 2 * time.Second
	assert.ErrorIs(t, s.ScanFile(context.Background(), path), apperrors.ErrAttachmentScanFailed)
}

// TestClamAVScannerZeroChunkWriteFailure verifica falha de scan ao escrever o chunk zero terminador.
func TestClamAVScannerZeroChunkWriteFailure(t *testing.T) {

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		reader := bufio.NewReader(conn)
		_, _ = reader.ReadString('\n')
		sizeBuf := make([]byte, 4)
		for {
			if _, readErr := io.ReadFull(reader, sizeBuf); readErr != nil {
				_ = conn.Close()
				return
			}
			n := binary.BigEndian.Uint32(sizeBuf)
			if n == 0 {
				return
			}
			_, _ = io.CopyN(io.Discard, reader, int64(n))
			_ = conn.Close() // fecha antes do cliente enviar o zero terminador, se ainda pendente
			return
		}
	}()

	path := filepath.Join(t.TempDir(), "file.bin")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))
	s := NewClamAVScanner(ln.Addr().String())
	s.timeout = 2 * time.Second
	assert.ErrorIs(t, s.ScanFile(context.Background(), path), apperrors.ErrAttachmentScanFailed)
}

type limitedWriteConn struct {
	net.Conn
	limit int
	wrote int
}

func (c *limitedWriteConn) Write(p []byte) (int, error) {
	if c.wrote >= c.limit {
		return 0, errors.New("write limit")
	}
	if c.wrote+len(p) > c.limit {
		return 0, errors.New("write limit")
	}
	n, err := c.Conn.Write(p)
	c.wrote += n
	return n, err
}

// TestClamAVScannerDialWriteHooks verifica falhas de escrita em cada etapa do protocolo INSTREAM via hook de dial.
func TestClamAVScannerDialWriteHooks(t *testing.T) {
	addr := startFakeClamAV(t, clamAVResponseOK)
	path := filepath.Join(t.TempDir(), "file.bin")
	require.NoError(t, os.WriteFile(path, bytesRepeat(1024), 0o600))

	orig := clamDialContext
	t.Cleanup(func() { clamDialContext = orig })

	instream := len("nINSTREAM\n")
	s := NewClamAVScanner(addr)
	s.timeout = 2 * time.Second

	for _, limit := range []int{
		0,                   // falha INSTREAM
		instream,            // falha no header de tamanho
		instream + 4,        // falha no payload
		instream + 4 + 1024, // falha no chunk zero terminador
	} {
		limit := limit
		clamDialContext = func(ctx context.Context, timeout time.Duration, network, address string) (net.Conn, error) {
			c, err := orig(ctx, timeout, network, address)
			if err != nil {
				return nil, err
			}
			return &limitedWriteConn{Conn: c, limit: limit}, nil
		}
		assert.ErrorIs(t, s.ScanFile(context.Background(), path), apperrors.ErrAttachmentScanFailed, "limit=%d", limit)
	}
}

type clamAVMode int

const (
	clamAVResponseOK clamAVMode = iota
	clamAVResponseFound
	clamAVResponseOther
)

func startFakeClamAV(t *testing.T, mode clamAVMode) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go handleFakeClamAV(conn, mode)
		}
	}()
	return ln.Addr().String()
}

func handleFakeClamAV(conn net.Conn, mode clamAVMode) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	if !strings.HasPrefix(line, "nINSTREAM") {
		_, _ = io.WriteString(conn, "UNKNOWN COMMAND\n")
		return
	}
	sizeBuf := make([]byte, 4)
	for {
		if _, err := io.ReadFull(reader, sizeBuf); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(sizeBuf)
		if n == 0 {
			break
		}
		if _, err := io.CopyN(io.Discard, reader, int64(n)); err != nil {
			return
		}
	}
	switch mode {
	case clamAVResponseFound:
		_, _ = io.WriteString(conn, "stream: Eicar-Test-Signature FOUND\n")
	case clamAVResponseOther:
		_, _ = io.WriteString(conn, "stream: ERROR\n")
	default:
		_, _ = io.WriteString(conn, "stream: OK\n")
	}
}

func bytesRepeat(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'A'
	}
	return b
}

type memFile struct {
	*bytes.Reader
}

func (memFile) Close() error { return nil }

// TestSanitizeAttachmentName verifica a sanitização do nome do anexo (path, controles e espaços).
func TestSanitizeAttachmentName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "arquivo.txt", sanitizeAttachmentName(`C:\tmp\arquivo.txt`))
	assert.Equal(t, "nomelimpo.pdf", sanitizeAttachmentName("nome\tlimpo.pdf"))
	assert.Equal(t, "nome limpo.pdf", sanitizeAttachmentName("nome   limpo.pdf"))
	assert.Equal(t, "", sanitizeAttachmentName("///"))
}

// TestAllowedAttachmentMime verifica o mapeamento de extensões permitidas e rejeição de .exe.
func TestAllowedAttachmentMime(t *testing.T) {
	t.Parallel()

	mime, ok := allowedAttachmentMime(".png")
	assert.True(t, ok)
	assert.Equal(t, "image/png", mime)

	_, ok = allowedAttachmentMime(".exe")
	assert.False(t, ok)
}

// TestShouldScanAttachment verifica quais extensões devem passar por varredura antivírus.
func TestShouldScanAttachment(t *testing.T) {
	t.Parallel()

	assert.True(t, shouldScanAttachment(".pdf"))
	assert.True(t, shouldScanAttachment(".DOCX"))
	assert.False(t, shouldScanAttachment(".txt"))
}

// TestValidateDetectedMime verifica validação de MIME detectado, tipo incompatível e arquivo vazio.
func TestValidateDetectedMime(t *testing.T) {
	t.Parallel()

	txt := memFile{Reader: bytes.NewReader([]byte("conteudo de texto simples"))}
	require.NoError(t, validateDetectedMime(txt, ".txt", int64(txt.Len())))

	bad := memFile{Reader: bytes.NewReader([]byte("%PDF-1.4"))}
	assert.ErrorIs(t, validateDetectedMime(bad, ".txt", int64(bad.Len())), apperrors.ErrAttachmentTypeNotAllowed)

	empty := memFile{Reader: bytes.NewReader(nil)}
	assert.ErrorIs(t, validateDetectedMime(empty, ".txt", 0), apperrors.ErrAttachmentEmpty)
}

// TestIsDocxPackage verifica detecção de pacote DOCX válido versus conteúdo não ZIP.
func TestIsDocxPackage(t *testing.T) {
	t.Parallel()

	plain := memFile{Reader: bytes.NewReader([]byte("not-a-zip"))}
	assert.False(t, isDocxPackage(plain, int64(plain.Len())))

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_, err := zw.Create("[Content_Types].xml")
	require.NoError(t, err)
	_, err = zw.Create("word/document.xml")
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	docx := memFile{Reader: bytes.NewReader(buf.Bytes())}
	assert.True(t, isDocxPackage(docx, int64(docx.Len())))
}

// TestLocalAttachmentStoragePathRejectsTraversal verifica que Path rejeita traversal e chave vazia.
func TestLocalAttachmentStoragePathRejectsTraversal(t *testing.T) {
	t.Parallel()

	storage := NewLocalAttachmentStorage(t.TempDir())
	_, err := storage.Path("..")
	assert.ErrorIs(t, err, apperrors.ErrAttachmentInvalidPath)

	_, err = storage.Path("")
	assert.ErrorIs(t, err, apperrors.ErrAttachmentInvalidPath)
}

// TestLocalAttachmentStorageSaveAndDelete verifica Save e Delete de anexo local no disco.
func TestLocalAttachmentStorageSaveAndDelete(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	storage := NewLocalAttachmentStorage(root, NoopAttachmentScanner{})
	content := []byte("conteudo de texto simples para anexo")
	file := memFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{Filename: "nota.txt", Size: int64(len(content))}

	stored, err := storage.Save(context.Background(), 7, 3, file, header)
	require.NoError(t, err)
	assert.Equal(t, "nota.txt", stored.OriginalName)
	assert.Equal(t, "text/plain", stored.MimeType)
	assert.Equal(t, int64(len(content)), stored.SizeBytes)
	assert.True(t, strings.HasPrefix(stored.StorageKey, "7/3/"))
	assert.True(t, strings.HasSuffix(stored.StorageKey, ".txt"))

	absPath, err := storage.Path(stored.StorageKey)
	require.NoError(t, err)
	_, err = os.Stat(absPath)
	require.NoError(t, err)
	rel, err := filepath.Rel(root, absPath)
	require.NoError(t, err)
	assert.False(t, strings.HasPrefix(rel, ".."))

	require.NoError(t, storage.Delete(context.Background(), stored.StorageKey))
	require.NoError(t, storage.Delete(context.Background(), stored.StorageKey))
}

// TestNoopAttachmentScanner verifica que NoopAttachmentScanner sempre retorna sucesso.
func TestNoopAttachmentScanner(t *testing.T) {
	t.Parallel()
	assert.NoError(t, NoopAttachmentScanner{}.ScanFile(context.Background(), "any"))
}

// TestRandomAttachmentFileName verifica geração de nome aleatório com extensão correta.
func TestRandomAttachmentFileName(t *testing.T) {
	t.Parallel()

	name, err := randomAttachmentFileName(".png")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(name, ".png"))
	assert.Len(t, name, 32+len(".png"))
}

// TestNewLocalAttachmentStorageDefaults verifica defaults de root e scanner em NewLocalAttachmentStorage.
func TestNewLocalAttachmentStorageDefaults(t *testing.T) {
	t.Parallel()

	storage := NewLocalAttachmentStorage("")
	assert.Equal(t, "storage/attachments", storage.root)
	assert.NotNil(t, storage.scanner)

	storage = NewLocalAttachmentStorage(" custom ", nil)
	assert.Equal(t, "custom", storage.root)
}

// TestSanitizeAttachmentNameLongAndControls verifica truncamento de nomes longos e extensão excessiva.
func TestSanitizeAttachmentNameLongAndControls(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 200) + ".txt"
	got := sanitizeAttachmentName(long)
	assert.LessOrEqual(t, len([]rune(got)), 160)
	assert.True(t, strings.HasSuffix(got, ".txt"))

	// extensão maior que o limite não deixa espaço para o basename
	assert.Equal(t, "", sanitizeAttachmentName(strings.Repeat("x", 200)+"."+strings.Repeat("y", 200)))
}

// TestAllowedAttachmentMimeAll verifica o MIME esperado para todas as extensões permitidas.
func TestAllowedAttachmentMimeAll(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".webp": "image/webp",
		".pdf":  "application/pdf",
		".txt":  "text/plain",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	}
	for ext, want := range cases {
		got, ok := allowedAttachmentMime(ext)
		assert.True(t, ok, ext)
		assert.Equal(t, want, got, ext)
	}
}

// TestValidateDetectedMimeAllTypes verifica MIME detectado para imagens, PDF, DOCX e erros de I/O.
func TestValidateDetectedMimeAllTypes(t *testing.T) {
	t.Parallel()

	jpegBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	require.NoError(t, validateDetectedMime(memFile{Reader: bytes.NewReader(jpegBytes)}, ".jpg", int64(len(jpegBytes))))
	require.NoError(t, validateDetectedMime(memFile{Reader: bytes.NewReader(jpegBytes)}, ".jpeg", int64(len(jpegBytes))))

	png := memFile{Reader: bytes.NewReader([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0})}
	require.NoError(t, validateDetectedMime(png, ".png", int64(png.Len())))

	webpBytes := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}
	webp := memFile{Reader: bytes.NewReader(webpBytes)}
	require.NoError(t, validateDetectedMime(webp, ".webp", int64(len(webpBytes))))

	pdf := memFile{Reader: bytes.NewReader([]byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n"))}
	require.NoError(t, validateDetectedMime(pdf, ".pdf", int64(pdf.Len())))

	docxBytes := mustDocxBytes(t)
	docx := memFile{Reader: bytes.NewReader(docxBytes)}
	require.NoError(t, validateDetectedMime(docx, ".docx", int64(len(docxBytes))))

	assert.ErrorIs(t, validateDetectedMime(memFile{Reader: bytes.NewReader([]byte("hello"))}, ".png", 5), apperrors.ErrAttachmentTypeNotAllowed)

	errReader := &errMemFile{readErr: errors.New("read boom")}
	assert.ErrorContains(t, validateDetectedMime(errReader, ".txt", 1), "read boom")

	seekFail := &errMemFile{data: []byte("conteudo de texto simples"), seekErr: errors.New("seek boom")}
	assert.ErrorContains(t, validateDetectedMime(seekFail, ".txt", 10), "seek boom")
}

// TestIsDocxPackageIncomplete verifica que ZIP incompleto sem word/document.xml não é DOCX.
func TestIsDocxPackageIncomplete(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_, err := zw.Create("[Content_Types].xml")
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	incomplete := memFile{Reader: bytes.NewReader(buf.Bytes())}
	assert.False(t, isDocxPackage(incomplete, int64(incomplete.Len())))
}

// TestLocalAttachmentStorageSaveValidationBranches verifica ramos de validação de Save (contexto, vazio, tamanho, nome e tipo).
func TestLocalAttachmentStorageSaveValidationBranches(t *testing.T) {
	t.Parallel()

	storage := NewLocalAttachmentStorage(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := storage.Save(ctx, 1, 1, memFile{Reader: bytes.NewReader([]byte("x"))}, &multipart.FileHeader{Filename: "a.txt", Size: 1})
	require.Error(t, err)

	_, err = storage.Save(context.Background(), 1, 1, nil, nil)
	assert.ErrorIs(t, err, apperrors.ErrAttachmentEmpty)

	_, err = storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader([]byte("x"))}, &multipart.FileHeader{Filename: "a.txt", Size: 0})
	assert.ErrorIs(t, err, apperrors.ErrAttachmentEmpty)

	_, err = storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(make([]byte, 10))}, &multipart.FileHeader{Filename: "a.txt", Size: MaxAttachmentSizeBytes + 1})
	assert.ErrorIs(t, err, apperrors.ErrAttachmentTooLarge)

	_, err = storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader([]byte("x"))}, &multipart.FileHeader{Filename: "///", Size: 1})
	assert.ErrorIs(t, err, apperrors.ErrAttachmentInvalidName)

	_, err = storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader([]byte("MZ"))}, &multipart.FileHeader{Filename: "a.exe", Size: 2})
	assert.ErrorIs(t, err, apperrors.ErrAttachmentTypeNotAllowed)

	_, err = storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader([]byte("%PDF-1.4"))}, &multipart.FileHeader{Filename: "a.txt", Size: 8})
	assert.ErrorIs(t, err, apperrors.ErrAttachmentTypeNotAllowed)
}

// TestLocalAttachmentStorageSaveWithScanner verifica Save com scanner e propagação de apperrors.ErrAttachmentMalicious.
func TestLocalAttachmentStorageSaveWithScanner(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	scanner := &recordingScanner{}
	storage := NewLocalAttachmentStorage(root, scanner)

	pdf := []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\ntrailer\n")
	stored, err := storage.Save(context.Background(), 2, 4, memFile{Reader: bytes.NewReader(pdf)}, &multipart.FileHeader{Filename: "doc.pdf", Size: int64(len(pdf))})
	require.NoError(t, err)
	assert.Equal(t, "application/pdf", stored.MimeType)
	assert.Equal(t, 1, scanner.calls)

	scanner.err = apperrors.ErrAttachmentMalicious
	_, err = storage.Save(context.Background(), 2, 4, memFile{Reader: bytes.NewReader(pdf)}, &multipart.FileHeader{Filename: "doc.pdf", Size: int64(len(pdf))})
	assert.ErrorIs(t, err, apperrors.ErrAttachmentMalicious)
}

// TestLocalAttachmentStorageSaveDocx verifica Save bem-sucedido de arquivo DOCX.
func TestLocalAttachmentStorageSaveDocx(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	storage := NewLocalAttachmentStorage(root, NoopAttachmentScanner{})
	docx := mustDocxBytes(t)
	stored, err := storage.Save(context.Background(), 1, 2, memFile{Reader: bytes.NewReader(docx)}, &multipart.FileHeader{Filename: "note.docx", Size: int64(len(docx))})
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(stored.StorageKey, ".docx"))
}

// TestLocalAttachmentStorageSaveImageTypes verifica Save de JPEG, PNG e WebP com MIME correto.
func TestLocalAttachmentStorageSaveImageTypes(t *testing.T) {
	t.Parallel()

	storage := NewLocalAttachmentStorage(t.TempDir())
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0, 0, 0, 0}
	stored, err := storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(jpeg)}, &multipart.FileHeader{Filename: "a.jpg", Size: int64(len(jpeg))})
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", stored.MimeType)

	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0, 0, 0, 0, 0}
	stored, err = storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(png)}, &multipart.FileHeader{Filename: "a.png", Size: int64(len(png))})
	require.NoError(t, err)
	assert.Equal(t, "image/png", stored.MimeType)

	webp := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}
	stored, err = storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(webp)}, &multipart.FileHeader{Filename: "a.webp", Size: int64(len(webp))})
	require.NoError(t, err)
	assert.Equal(t, "image/webp", stored.MimeType)
}

// TestLocalAttachmentStorageSaveEmptyAfterSeek verifica rejeição quando o arquivo fica vazio após seek.
func TestLocalAttachmentStorageSaveEmptyAfterSeek(t *testing.T) {
	t.Parallel()

	storage := NewLocalAttachmentStorage(t.TempDir())
	content := []byte("conteudo de texto simples")
	file := &seekToEndFile{data: content}
	_, err := storage.Save(context.Background(), 1, 1, file, &multipart.FileHeader{Filename: "a.txt", Size: int64(len(content))})
	assert.ErrorIs(t, err, apperrors.ErrAttachmentEmpty)
}

// TestLocalAttachmentStorageSaveRenameFailure verifica falha de Save quando o rename para o destino final falha.
func TestLocalAttachmentStorageSaveRenameFailure(t *testing.T) {
	orig := cryptoRandRead
	t.Cleanup(func() { cryptoRandRead = orig })
	cryptoRandRead = func(b []byte) (int, error) {
		for i := range b {
			b[i] = 0xab
		}
		return len(b), nil
	}

	root := t.TempDir()
	storage := NewLocalAttachmentStorage(root)
	fileName, err := randomAttachmentFileName(".txt")
	require.NoError(t, err)
	finalDir := filepath.Join(root, "1", "1", fileName)
	require.NoError(t, os.MkdirAll(finalDir, 0o700))

	content := []byte("conteudo de texto simples")
	_, err = storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(content)}, &multipart.FileHeader{Filename: "a.txt", Size: int64(len(content))})
	require.Error(t, err)
}

// TestLocalAttachmentStorageCreateTempAndCloseErrors verifica falhas ao criar ou fechar o arquivo temporário no Save.
func TestLocalAttachmentStorageCreateTempAndCloseErrors(t *testing.T) {
	orig := osCreateTemp
	t.Cleanup(func() { osCreateTemp = orig })

	storage := NewLocalAttachmentStorage(t.TempDir())
	content := []byte("conteudo de texto simples")
	osCreateTemp = func(string, string) (uploadTempFile, error) {
		return nil, errors.New("create temp boom")
	}
	_, err := storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(content)}, &multipart.FileHeader{Filename: "a.txt", Size: int64(len(content))})
	require.ErrorContains(t, err, "create temp boom")

	real, err := os.CreateTemp(t.TempDir(), "close-*")
	require.NoError(t, err)
	osCreateTemp = func(string, string) (uploadTempFile, error) {
		return &badCloseTemp{File: real}, nil
	}
	_, err = storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(content)}, &multipart.FileHeader{Filename: "a.txt", Size: int64(len(content))})
	require.ErrorContains(t, err, "close boom")
}

type badCloseTemp struct{ *os.File }

func (f *badCloseTemp) Close() error {
	_ = f.File.Close()
	return errors.New("close boom")
}

// TestLocalAttachmentStorageDeleteRemoveError verifica comportamento de Delete quando a remoção do arquivo falha.
func TestLocalAttachmentStorageDeleteRemoveError(t *testing.T) {
	root := t.TempDir()
	storage := NewLocalAttachmentStorage(root)
	content := []byte("conteudo de texto simples")
	stored, err := storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(content)}, &multipart.FileHeader{Filename: "a.txt", Size: int64(len(content))})
	require.NoError(t, err)

	abs, err := storage.Path(stored.StorageKey)
	require.NoError(t, err)
	f, err := os.OpenFile(abs, os.O_RDWR, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	err = storage.Delete(context.Background(), stored.StorageKey)
	if err != nil {
		assert.NotErrorIs(t, err, apperrors.ErrAttachmentInvalidPath)
		return
	}
	// Alguns SOs permitem apagar um arquivo aberto; trate como aceitável.
	t.Log("delete of open file succeeded on this OS")
}

// TestLocalAttachmentStorageSaveCopyErrors verifica falha de Save quando a cópia do conteúdo falha.
func TestLocalAttachmentStorageSaveCopyErrors(t *testing.T) {
	t.Parallel()

	storage := NewLocalAttachmentStorage(t.TempDir())
	content := []byte("conteudo de texto simples")
	file := &errMemFile{data: content, failAfterReads: 1, readErr: errors.New("copy boom")}
	_, err := storage.Save(context.Background(), 1, 1, file, &multipart.FileHeader{Filename: "a.txt", Size: int64(len(content))})
	require.ErrorContains(t, err, "copy boom")
}

// TestLocalAttachmentStorageSaveMkdirFailure verifica falha de Save quando não é possível criar o diretório de destino.
func TestLocalAttachmentStorageSaveMkdirFailure(t *testing.T) {
	t.Parallel()

	rootFile := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(rootFile, []byte("x"), 0o600))
	storage := NewLocalAttachmentStorage(rootFile)
	content := []byte("conteudo de texto simples")
	_, err := storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(content)}, &multipart.FileHeader{Filename: "a.txt", Size: int64(len(content))})
	require.Error(t, err)
}

// TestLocalAttachmentStorageSaveTooLargeOnCopy verifica rejeição quando o payload excede o tamanho máximo na cópia.
func TestLocalAttachmentStorageSaveTooLargeOnCopy(t *testing.T) {
	t.Parallel()

	storage := NewLocalAttachmentStorage(t.TempDir())
	// tamanho do header dentro do limite; o payload real excede MaxAttachmentSizeBytes via LimitReader
	payload := []byte(strings.Repeat("conteudo de texto simples\n", (MaxAttachmentSizeBytes/26)+4))
	_, err := storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(payload)}, &multipart.FileHeader{
		Filename: "a.txt",
		Size:     26,
	})
	assert.ErrorIs(t, err, apperrors.ErrAttachmentTooLarge)
}

// TestLocalAttachmentStoragePathOK verifica Path bem-sucedido para chave relativa válida.
func TestLocalAttachmentStoragePathOK(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	storage := NewLocalAttachmentStorage(root)
	abs, err := storage.Path("7/3/file.txt")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(abs), "7/3/file.txt") || strings.Contains(abs, filepath.Join("7", "3", "file.txt")))
}

// TestLocalAttachmentStorageDeleteBranches verifica Delete com contexto cancelado, chave inválida e sucesso.
func TestLocalAttachmentStorageDeleteBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	storage := NewLocalAttachmentStorage(root)
	content := []byte("conteudo de texto simples")
	stored, err := storage.Save(context.Background(), 1, 1, memFile{Reader: bytes.NewReader(content)}, &multipart.FileHeader{Filename: "a.txt", Size: int64(len(content))})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, storage.Delete(ctx, stored.StorageKey))

	require.ErrorIs(t, storage.Delete(context.Background(), ""), apperrors.ErrAttachmentInvalidPath)
	require.NoError(t, storage.Delete(context.Background(), stored.StorageKey))
}

type recordingScanner struct {
	calls int
	err   error
}

func (s *recordingScanner) ScanFile(context.Context, string) error {
	s.calls++
	return s.err
}

type errMemFile struct {
	data           []byte
	off            int
	reads          int
	failAfterReads int
	readErr        error
	seekErr        error
}

func (f *errMemFile) Read(p []byte) (int, error) {
	f.reads++
	if f.readErr != nil && f.failAfterReads > 0 && f.reads > f.failAfterReads {
		return 0, f.readErr
	}
	if f.readErr != nil && f.failAfterReads == 0 {
		return 0, f.readErr
	}
	if f.off >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.off:])
	f.off += n
	return n, nil
}

func (f *errMemFile) Seek(offset int64, whence int) (int64, error) {
	if f.seekErr != nil {
		return 0, f.seekErr
	}
	switch whence {
	case io.SeekStart:
		f.off = int(offset)
	case io.SeekCurrent:
		f.off += int(offset)
	case io.SeekEnd:
		f.off = len(f.data) + int(offset)
	}
	return int64(f.off), nil
}

func (f *errMemFile) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || int(off) >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *errMemFile) Close() error { return nil }

func mustDocxBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_, err := zw.Create("[Content_Types].xml")
	require.NoError(t, err)
	_, err = zw.Create("word/document.xml")
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// seekToEndFile passa na detecção de MIME e depois posiciona no EOF para que Copy não escreva nada.
type seekToEndFile struct {
	data []byte
	off  int
}

func (f *seekToEndFile) Read(p []byte) (int, error) {
	if f.off >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.off:])
	f.off += n
	return n, nil
}

func (f *seekToEndFile) Seek(offset int64, whence int) (int64, error) {
	f.off = len(f.data)
	return int64(f.off), nil
}

func (f *seekToEndFile) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || int(off) >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *seekToEndFile) Close() error { return nil }
