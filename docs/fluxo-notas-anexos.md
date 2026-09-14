# Fluxo de notas com anexos

Este documento descreve como o Bridopen salva uma nota e como o fluxo de anexos foi implementado no código atual.

## Decisão principal

O arquivo não é salvo no PostgreSQL. O banco guarda apenas metadados do anexo e a chave/caminho de armazenamento.

O fluxo foi desenhado para evitar anexos órfãos antes da nota existir:

1. O usuário cria a nota normalmente.
2. A nota é salva no banco e recebe um `id`.
3. Depois disso, a tela da nota libera o envio de anexos.
4. O upload usa a rota `POST /note/{id}/attachments`.

Na tela `/note/new`, o painel de anexos aparece apenas como informativo. O upload só fica ativo depois que a nota já existe.

## Arquivos principais

| Camada | Arquivo | Responsabilidade |
| --- | --- | --- |
| Rotas | `cmd/http/routes.go` | Registra as rotas de nota e anexos. |
| Config | `cmd/http/config.go` | Lê `BRIDOPEN_ATTACHMENT_STORAGE_DIR`. |
| Bootstrap | `cmd/http/main.go` | Passa o diretório de anexos para as rotas. |
| Handler | `internal/handlers/notes/handler/handler.go` | Recebe requests, valida dono da nota, chama storage e repositório. |
| Storage | `internal/handlers/notes/service/attachment_storage.go` | Salva, resolve caminho e remove arquivos físicos. |
| Repositório | `internal/handlers/notes/repositories/repo.go` | Persiste metadados dos anexos e carrega anexos da nota. |
| Model | `internal/handlers/notes/model/model.go` | Define `NoteAttachment`. |
| DTO | `internal/handlers/notes/dto/dto.go` | Converte anexos para dados de tela. |
| Template | `views/templates/partials/note-attachments.html` | Painel lateral de upload/listagem/remoção. |
| JS | `views/static/js/notes/note-delete-modal.js` | Confirma e remove anexos via `DELETE` com CSRF. |
| Migration | `db/migrations/000002_create_notes_table.up.sql` | Cria `note_attachments`. |

## Estrutura de armazenamento

Por padrão, em desenvolvimento:

```text
storage/attachments/{user_id}/{note_id}/{uuid.ext}
```

Exemplo:

```text
storage/attachments/7/42/8f4d2c0f98ab4b8d91a1c48153fd8f6a.pdf
```

A configuração vem de:

```env
BRIDOPEN_ATTACHMENT_STORAGE_DIR=storage/attachments
```

No `docker-compose.yml`, o servidor usa:

```yaml
BRIDOPEN_ATTACHMENT_STORAGE_DIR: ${BRIDOPEN_ATTACHMENT_STORAGE_DIR:-/data/attachments}
volumes:
  - attachments_data:/data/attachments
```

Assim, em produção com Docker, os arquivos ficam em um volume persistente chamado `attachments_data`.

## Tabela `note_attachments`

A tabela fica na migration `db/migrations/000002_create_notes_table.up.sql`.

Campos principais:

| Campo | Uso |
| --- | --- |
| `id` | Identificador do anexo. |
| `note_id` | Nota dona do anexo. |
| `user_id` | Usuário dono do anexo. |
| `original_name` | Nome original sanitizado para exibição/download. |
| `storage_key` | Caminho relativo usado pelo storage. |
| `mime_type` | MIME validado do arquivo. |
| `size_bytes` | Tamanho real salvo. |
| `checksum_sha256` | Hash SHA-256 do arquivo salvo. |
| `created_at` | Data de envio. |

Constraint importante:

```sql
foreign key (note_id, user_id)
references public.notes(id, user_id)
on delete cascade
```

Isso garante que o anexo pertence a uma nota daquele usuário. Também existe `unique(storage_key)` para impedir duas linhas apontando para o mesmo arquivo.

## Fluxo 1: criar nota

### 1. Usuário abre `/note/new`

Rota:

```go
GET /note/new
```

Handler:

```go
NoteNew
```

O handler carrega as etiquetas disponíveis e renderiza `note-new.html` com um `NoteRequest` novo.

O template `note-new.html` inclui:

```gotemplate
{{ template "note-attachments" . }}
```

Como a nota ainda não tem `Id`, o partial mostra a mensagem de que é preciso salvar a nota antes de anexar arquivos.

### 2. Usuário salva a nota

Rota:

```go
POST /note
```

Handler:

```go
NoteSave
```

O handler faz:

1. `ParseForm()`.
2. Lê `id`, `title`, `content`, `color` e `tag_ids`.
3. Valida título, conteúdo e etiquetas.
4. Se `id == 0`, chama `repo.Create(...)`.
5. Se `id > 0`, chama `repo.Update(...)`.
6. Redireciona para `/note/{id}`.

No `repo.Create(...)`, a criação da nota acontece em transação:

1. Insere em `public.notes` e recebe o `id`.
2. Insere em `public.notes_content` usando esse `id`.
3. Sincroniza etiquetas, se houver.
4. Faz `commit`.

Depois disso a nota existe oficialmente e já pode receber anexos.

## Fluxo 2: abrir nota já salva

Rota:

```go
GET /note/{id}
```

Handler:

```go
NoteView
```

O handler chama:

```go
repo.GetById(ctx, userID, noteID)
```

O repositório busca a nota e depois carrega dados relacionados:

1. `loadTagsForNotes(...)`
2. `loadAttachmentsForNotes(...)`

O DTO converte os anexos para a tela com:

```go
NewNoteAttachmentResponseList(...)
```

Cada anexo ganha:

- `Id`
- `Name`
- `MimeType`
- `SizeLabel`
- `URL`
- `IsImage`

A URL fica neste formato:

```text
/note/{noteId}/attachments/{attachmentId}
```

## Fluxo 3: enviar anexo

### 1. Front envia multipart

O partial `note-attachments.html` renderiza um form:

```html
<form action="/note/{{ .Id }}/attachments" method="post" enctype="multipart/form-data">
```

Campos importantes:

- `csrfField`
- `redirect`
- `attachment`

O campo `redirect` permite voltar para `/note/{id}` ou `/note/{id}/edit` depois do upload.

### 2. Handler recebe upload

Rota:

```go
POST /note/{id}/attachments
```

Handler:

```go
NoteAttachmentUpload
```

Passos do handler:

1. Lê `noteID` da URL.
2. Lê `userID` da sessão.
3. Aplica `http.MaxBytesReader` para limitar o corpo da request.
4. Chama `ParseMultipartForm`.
5. Busca a nota com `repo.GetById(ctx, userID, noteID)`.
6. Se a nota não pertence ao usuário, retorna 404.
7. Se a nota está na lixeira, bloqueia upload.
8. Lê o arquivo com `r.FormFile("attachment")`.
9. Chama `attachmentStorage.Save(...)`.
10. Chama `repo.CreateAttachment(...)` para salvar metadados.
11. Registra auditoria.
12. Redireciona para a página anterior.

### 3. Storage salva arquivo físico

Arquivo:

```go
internal/handlers/notes/service/attachment_storage.go
```

Método:

```go
Save(ctx, userID, noteID, file, header)
```

Validações aplicadas:

| Regra | Como é validada |
| --- | --- |
| Arquivo obrigatório | `file` e `header` não podem ser nulos. |
| Arquivo vazio | `header.Size > 0` e bytes escritos > 0. |
| Tamanho máximo | `MaxAttachmentSizeBytes = 10 << 20`, ou seja, 10 MB. |
| Nome seguro | `sanitizeAttachmentName(...)`. |
| Extensão permitida | `.jpg`, `.jpeg`, `.png`, `.webp`, `.pdf`, `.txt`, `.docx`. |
| MIME real | `http.DetectContentType(...)`. |
| DOCX real | Abre como ZIP e procura `[Content_Types].xml` e `word/document.xml`. |
| Path traversal | A chave é resolvida com validação em `Path(...)`. |

O storage gera um nome aleatório:

```go
randomAttachmentFileName(ext)
```

Depois cria o diretório:

```text
{root}/{userID}/{noteID}
```

O arquivo é escrito primeiro como temporário e depois renomeado para o caminho final. Durante a cópia, o código calcula o SHA-256.

Retorno do storage:

```go
StoredAttachment{
    OriginalName,
    StorageKey,
    MimeType,
    SizeBytes,
    ChecksumSHA256,
}
```

### 4. Repositório salva metadados

Método:

```go
CreateAttachment(ctx, userID, noteID, input)
```

Esse método usa transação.

Passos:

1. Abre transação.
2. Busca a nota com `for update`.
3. Garante que a nota pertence ao usuário.
4. Garante que `deleted_at is null`.
5. Conta quantos anexos a nota já tem.
6. Se já tiver 10, retorna `ErrAttachmentLimit`.
7. Insere em `public.note_attachments`.
8. Faz `commit`.

Se o insert de metadados falhar depois do arquivo físico ter sido salvo, o handler tenta remover o arquivo físico para não deixar lixo no storage.

## Fluxo 4: baixar ou visualizar anexo

Rota:

```go
GET /note/{id}/attachments/{attachmentId}
```

Handler:

```go
NoteAttachmentDownload
```

Passos:

1. Lê `noteID` e `attachmentID` da URL.
2. Lê `userID` da sessão.
3. Busca o anexo com `repo.GetAttachment(ctx, userID, noteID, attachmentID)`.
4. Se não existir para aquele usuário/nota, retorna 404.
5. Resolve o caminho físico com `attachmentStorage.Path(storageKey)`.
6. Define headers:
   - `Content-Type`
   - `Content-Disposition`
   - `X-Content-Type-Options: nosniff`
7. Envia com `http.ServeFile`.

Arquivos exibidos inline:

- imagens
- PDF
- TXT

Outros tipos são tratados como download.

## Fluxo 5: remover anexo

### 1. Front chama DELETE

Arquivo:

```text
views/static/js/notes/note-delete-modal.js
```

O botão de remover tem:

```html
data-note-delete-action
data-note-delete-url="/note/{noteId}/attachments/{attachmentId}"
data-note-delete-reload="true"
```

O JS faz `fetch` com:

```http
DELETE /note/{id}/attachments/{attachmentId}
X-CSRF-Token: ...
```

Se der certo, a página recarrega.

### 2. Handler remove metadados e arquivo

Handler:

```go
NoteAttachmentDelete
```

Passos:

1. Lê `noteID`, `attachmentID` e `userID`.
2. Chama `repo.DeleteAttachment(...)`.
3. O repositório remove apenas se:
   - o anexo pertence à nota,
   - a nota pertence ao usuário,
   - a nota não está deletada.
4. O repositório retorna os metadados removidos.
5. O handler usa `storage_key` para remover o arquivo físico.
6. Retorna `204 No Content`.

Se a remoção física falhar, o erro é registrado em log como warning. A linha do banco já foi removida.

## Fluxo 6: excluir nota definitivamente

Handler:

```go
NoteDeletePermanently
```

Antes de apagar a nota, o handler lista os anexos:

```go
repo.ListAttachments(ctx, userID, noteID)
```

Depois:

1. Chama `repo.DeletePermanently(...)`.
2. Como existe `on delete cascade`, os metadados de anexos são removidos do banco.
3. O handler percorre a lista de anexos e remove os arquivos físicos pelo `storage_key`.

Se algum arquivo físico não for removido, o sistema registra warning no log.

## Tipos permitidos

Atualmente o sistema aceita:

| Extensão | MIME esperado |
| --- | --- |
| `.jpg`, `.jpeg` | `image/jpeg` |
| `.png` | `image/png` |
| `.webp` | `image/webp` |
| `.pdf` | `application/pdf` |
| `.txt` | `text/plain` |
| `.docx` | `application/vnd.openxmlformats-officedocument.wordprocessingml.document` |

Tamanho máximo por arquivo:

```text
10 MB
```

Limite por nota:

```text
10 anexos
```

## Pontos de segurança já cobertos

- Upload exige usuário autenticado pela rota protegida.
- CSRF continua ativo no form e no `DELETE` via JS.
- O anexo só é salvo se a nota pertence ao usuário logado.
- O anexo só é baixado se pertence à nota e ao usuário logado.
- O anexo só é removido se pertence à nota e ao usuário logado.
- O sistema não confia apenas na extensão do arquivo.
- O nome original é sanitizado.
- O arquivo é salvo com nome aleatório, não com nome enviado pelo usuário.
- O caminho físico é validado para evitar path traversal.
- O arquivo não fica dentro do banco.
- O upload antes da nota existir foi evitado.

## Pontos de atenção

1. Se o arquivo físico falhar ao ser removido depois de excluir metadados, pode sobrar arquivo no disco. Hoje isso fica registrado em log.
2. Não existe ainda limpeza automática de arquivos órfãos.
3. Não existe ainda limite total de armazenamento por usuário.
4. O upload atual é de um arquivo por vez.
5. Para migrar para S3, R2 ou MinIO, a ideia é criar outra implementação da interface `AttachmentStorage` sem mudar o handler.

## Como testar manualmente

1. Recrie ou atualize o banco com a migration que contém `note_attachments`.
2. Suba o servidor.
3. Acesse `/note/new`.
4. Crie uma nota normalmente.
5. Depois do redirect para `/note/{id}`, use o painel lateral `Anexos`.
6. Envie um arquivo permitido com até 10 MB.
7. Confirme que o arquivo apareceu na lista.
8. Clique no anexo para abrir/baixar.
9. Remova o anexo pelo ícone de lixeira.
10. Confira se a linha saiu do banco e se o arquivo físico saiu de `storage/attachments`.

Consulta útil no banco:

```sql
select
    id,
    note_id,
    user_id,
    original_name,
    storage_key,
    mime_type,
    size_bytes,
    checksum_sha256,
    created_at
from public.note_attachments
order by created_at desc;
```

## Resumo do desenho

O desenho atual é:

```text
Usuário salva nota
    -> POST /note
    -> repo.Create/Update
    -> redirect /note/{id}

Usuário anexa arquivo
    -> POST /note/{id}/attachments
    -> handler valida nota e usuário
    -> storage salva arquivo físico
    -> repo salva metadados em note_attachments
    -> redirect para view/edit

Usuário abre anexo
    -> GET /note/{id}/attachments/{attachmentId}
    -> repo valida dono
    -> storage resolve caminho
    -> ServeFile

Usuário remove anexo
    -> DELETE /note/{id}/attachments/{attachmentId}
    -> repo remove metadados
    -> storage remove arquivo físico
```
