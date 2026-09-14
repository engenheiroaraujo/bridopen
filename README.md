# Bridopen

Bridopen é uma aplicação web server-side rendered para anotações rápidas, com autenticação, verificação em duas etapas, anexos, tags, arquivamento, lixeira e páginas legais. O backend é um monolito modular em Go, organizado por camadas para manter handlers, services, models, repositories e infraestrutura com responsabilidades claras.

> Projeto opensource, distribuído sob licença MIT. Ele é entregue sem nenhuma
> instância oficial hospedada: quem subir o Bridopen é o responsável pelo
> serviço, pelos dados dos usuários e pelos textos legais das páginas públicas
> (privacidade, termos e cookies), que são modelos genéricos e precisam de
> revisão jurídica antes de ir ao ar.

## Tecnologias

| Camada | Tecnologia |
|--------|------------|
| Backend | Go 1.27, `net/http` nativo |
| Banco de dados | PostgreSQL com `pgx/v5` |
| Sessões | `alexedwards/scs` com store PostgreSQL |
| Templates | `html/template` com renderização server-side |
| Frontend | Tailwind CSS via CDN e JavaScript puro |
| Segurança | CSRF, cookies seguros, headers HTTP, rate limit, captcha e 2FA |
| Anexos | Storage local com scanner opcional via ClamAV |
| E-mail | SMTP via `gomail` |
| Migrations | `golang-migrate` |
| Containers | Docker Compose |
| Testes | `go test` com `testify` |

## Arquitetura

O projeto segue um monolito modular em camadas. O `cmd/http` é a composition root: carrega configuração, banco, sessão, mailer, storage, scanner, repositories, services, handlers, middlewares e rotas.

Regras principais:

- Handlers cuidam de HTTP, sessão, request/response, render e JSON.
- Services concentram casos de uso, validações de fluxo e orquestração.
- Models guardam entidades, estados, constantes e regras puras de domínio.
- Repositories isolam SQL, scans, transações e mapeamento de persistência.
- `internal/platform` guarda infraestrutura e preocupações transversais.
- Templates recebem DTOs/page models, não regras de negócio.

## Estrutura

```text
bridopen/
  cmd/
    http/
      main.go              # Inicialização do servidor, config, DB e middlewares
      routes.go            # Wiring dos módulos e registro das rotas
    exp/                   # Experimentos de código fora do fluxo principal
  db/
    migrations/            # Migrations SQL versionadas
  internal/
    handlers/
      legal/               # Páginas públicas: privacidade, termos e cookies
      notes/
        dto/               # Page models e DTOs das telas de notas
        handler/           # Handlers HTTP de notas, tags, anexos e estados
        model/             # Entidade Note e regras de domínio
        repositories/      # SQL e persistência de notas, tags e anexos
        service/           # Storage, scanner e casos técnicos de anexos
      users/
        dto/               # DTOs e page models de autenticação/conta
        handler/           # Handlers de auth, conta, perfil, senha e 2FA
        model/             # Entidades e estruturas de usuário
        repositories/      # SQL de usuários, sessões, 2FA e recovery codes
        service/           # Serviços de conta, validações e 2FA
    platform/
      audit/               # Auditoria de eventos de negócio
      captcha/             # Geração e validação de captcha
      config/              # Leitura e validação de variáveis de ambiente
      errors/              # Erros tipados de aplicação/repository/HTTP
      key/                 # Helpers de chaves, sessão e IP do cliente
      log/                 # Logger estruturado com slog
      mailer/              # Implementações de envio de e-mail
      middleware/          # Auth, access log e tratamento de erro HTTP
      password/            # Hash e comparação de senhas
      render/              # Renderização HTML e templates de e-mail
      security/            # CSRF, headers de segurança e rate limit
      validations/         # Validadores reutilizáveis de borda
  views/
    static/
      css/                 # Estilos globais
      img/                 # Logos e favicon
      js/
        core/              # Tema, navbar, toast e helpers globais
        notes/             # Interações das telas de notas
        account/           # Modais e ações de conta/segurança
    templates/
      base.html            # Layout principal autenticado
      base-auth.html       # Layout de autenticação
      pages/               # Páginas HTML
      partials/            # Componentes reutilizáveis
      mails/               # Templates HTML de e-mail
  docs/                    # Documentação auxiliar
  docker-compose.yml       # Produção
  docker-compose.local.yml # Banco e MailHog para desenvolvimento local
  docker-compose.dev.yml   # Ambiente de desenvolvimento conteinerizado
  Makefile
```

## Rotas

Principais grupos de rotas:

| Grupo | Rotas | Responsabilidade |
|-------|-------|------------------|
| Notas | `/`, `/note/new`, `/note/{id}`, `/notes/archive`, `/notes/trash` | Criar, listar, visualizar, editar, arquivar, restaurar e excluir notas |
| Anexos | `/note/{id}/attachments` | Upload, download inline quando seguro e remoção de anexos |
| Tags | `/tags`, `/tags/{id}` | Listagem, criação, edição e exclusão de tags |
| Autenticação | `/user/signup`, `/user/signin`, `/user/signout`, `/user/twofactor` | Cadastro, login, logout e login com 2FA |
| Recuperação | `/user/forgetpassword`, `/user/password/{token}`, `/user/resend` | Recuperação de senha e reenvio de confirmação |
| Conta | `/me`, `/me/security`, `/account/export`, `/account/delete` | Perfil, senha, e-mail de recuperação, sessões, 2FA, exportação e exclusão de conta |
| Legal | `/privacy`, `/terms`, `/cookies` | Políticas públicas |
| Estáticos | `/static/` | CSS, JavaScript e imagens |

## Variáveis de Ambiente

Use `.env.example` como base para criar o `.env` local. O `.env` não deve ser versionado.

```env
BRIDOPEN_APP_ENV=development
BRIDOPEN_SERVER_PORT=5000
BRIDOPEN_BASE_URL=http://localhost:5000
BRIDOPEN_DB_CONN_URL=postgres://postgres:postgres@localhost:5434/postgres?sslmode=disable
BRIDOPEN_LEVEL_LOG=info

BRIDOPEN_MAIL_HOST=localhost
BRIDOPEN_MAIL_PORT=1025
BRIDOPEN_MAIL_USERNAME=
BRIDOPEN_MAIL_PASSWORD=
BRIDOPEN_MAIL_FROM=nao-responder@exemplo.local

BRIDOPEN_CSRF_KEY=troque-esta-chave-de-desenvolvimento-por-uma-aleatoria
BRIDOPEN_MFA_SECRET_KEY=
BRIDOPEN_COOKIE_SECURE=false
BRIDOPEN_COOKIE_DOMAIN=
BRIDOPEN_TRUSTED_ORIGINS=localhost:5000,127.0.0.1:5000
BRIDOPEN_ATTACHMENT_STORAGE_DIR=storage/attachments
BRIDOPEN_CLAMAV_ADDR=
```

| Variável | Padrão | Obrigatória | Observação |
|----------|--------|-------------|------------|
| `BRIDOPEN_APP_ENV` | `development` | Não | Use `production` em produção |
| `BRIDOPEN_SERVER_PORT` | `5000` | Não | Porta HTTP do servidor |
| `BRIDOPEN_BASE_URL` | `http://localhost:5000` | Não | Em produção deve usar HTTPS |
| `BRIDOPEN_DB_CONN_URL` | - | Sim | URL de conexão PostgreSQL |
| `BRIDOPEN_LEVEL_LOG` | `info` | Não | Nível do `slog` |
| `BRIDOPEN_MAIL_HOST` | - | Sim | Host SMTP |
| `BRIDOPEN_MAIL_PORT` | - | Sim | Porta SMTP |
| `BRIDOPEN_MAIL_USERNAME` | - | Sim | Usuário SMTP, pode ser vazio em ambiente local controlado |
| `BRIDOPEN_MAIL_PASSWORD` | - | Sim | Senha SMTP, pode ser vazia em ambiente local controlado |
| `BRIDOPEN_MAIL_FROM` | `nao-responder@bridopen.local` | Não | Remetente padrão |
| `BRIDOPEN_CSRF_KEY` | - | Sim | Chave com pelo menos 32 caracteres em produção |
| `BRIDOPEN_MFA_SECRET_KEY` | - | Sim em produção | Segredo dedicado para 2FA |
| `BRIDOPEN_COOKIE_SECURE` | `false` | Não | Deve ser `true` em produção |
| `BRIDOPEN_COOKIE_DOMAIN` | - | Não | Domínio do cookie, quando necessário |
| `BRIDOPEN_TRUSTED_ORIGINS` | - | Não | Origens confiáveis para CSRF |
| `BRIDOPEN_ATTACHMENT_STORAGE_DIR` | `storage/attachments` | Não | Em produção, prefira volume persistente como `/data/attachments` |
| `BRIDOPEN_CLAMAV_ADDR` | - | Não | Endereço do ClamAV, exemplo `clamav:3310` |

Em produção, configure `BRIDOPEN_APP_ENV=production`, `BRIDOPEN_BASE_URL` com HTTPS, `BRIDOPEN_COOKIE_SECURE=true`, `BRIDOPEN_CSRF_KEY` forte, `BRIDOPEN_MFA_SECRET_KEY` forte e independente, storage persistente para anexos e ClamAV ativo quando aceitar uploads.

## Como Executar

### Pré-requisitos

- Go 1.27+
- Docker
- GNU Make
- `golang-migrate`

### 1. Subir dependências locais

```bash
make database
```

Esse comando usa `docker-compose.local.yml` e sobe PostgreSQL na porta `5434` e MailHog nas portas `1025` e `8025`.

### 2. Rodar migrations

No PowerShell:

```powershell
$env:DB_CONN_URL = "postgres://postgres:postgres@localhost:5434/postgres?sslmode=disable"
make migrate-up
```

### 3. Configurar `.env`

Crie o arquivo `.env` a partir do `.env.example` e ajuste `BRIDOPEN_DB_CONN_URL`, chaves e SMTP conforme o ambiente.

### 4. Iniciar servidor

```bash
make server
```

Ou diretamente:

```bash
go run ./cmd/http/.
```

Acesse: [http://localhost:5000](http://localhost:5000)

## Testes

Execute a suíte completa:

```bash
go test -count=1 ./...
```

No PowerShell também pode usar:

```powershell
go test -count=1 .\...
```

## Anexos e Segurança de Upload

Os anexos são gravados fora do banco, no diretório configurado por `BRIDOPEN_ATTACHMENT_STORAGE_DIR`. Em desenvolvimento o padrão é `storage/attachments`; em produção o Docker Compose usa volume persistente em `/data/attachments`.

Quando `BRIDOPEN_CLAMAV_ADDR` está configurado, uploads são verificados pelo ClamAV antes de serem persistidos. Arquivos bloqueados pela verificação de segurança não devem ser gravados nem disponibilizados ao usuário.

## Frontend

O projeto não usa SPA nem bundler. O JavaScript é dividido por responsabilidade:

| Pasta | Responsabilidade |
|-------|------------------|
| `views/static/js/core/` | Tema, navbar, toast global, trava de submit e helpers compartilhados |
| `views/static/js/notes/` | Busca, menus, cores, tags, contador de título, anexos e exclusão de notas |
| `views/static/js/account/` | Modais e ações de conta, senha, e-mail de recuperação e 2FA |

Feedback de ações comuns deve usar o toast/snackbar global. Modais ficam reservados para confirmações importantes ou ações destrutivas. Erros de campo continuam próximos dos campos no formulário.

## Comandos Make

```bash
make build-up       # Compila ./cmd/http/.
make server         # Roda o servidor local
make database       # Sobe PostgreSQL e MailHog locais
make migrate-create name=create_example
make migrate-up     # Aplica migrations; requer DB_CONN_URL
make migrate-down   # Reverte migrations; requer DB_CONN_URL
make exp            # Roda experimentos em cmd/exp

make prod-config    # Valida compose de produção
make prod-db        # Sobe banco de produção
make prod-migrate   # Executa migrations em produção
make prod-up        # Sobe server e Caddy em produção
make prod-deploy    # Deploy completo com prune de imagens
make prod-ps        # Lista containers
make prod-logs      # Logs do server
make prod-caddy-logs
make prod-db-logs
make prod-down      # Derruba containers sem apagar volumes
make prod-reset     # Derruba containers e apaga volumes; remove banco e anexos persistidos em volume
```

## Versionamento e releases

O Bridopen usa tags Git no formato `vMAJOR.MINOR.PATCH` como fonte oficial da
versão. Builds locais exibem `dev`; builds de produção recebem automaticamente
a tag, o commit e a data de compilação.

Para publicar uma versão, atualize a `main` e crie uma tag anotada:

```bash
git switch main
git pull
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

O push da tag executa os testes no CI. Este repositório não inclui workflow de
deploy: a publicação em produção fica a cargo de quem hospeda a instância, usando
os alvos `make prod-*` ou o pipeline de sua preferência. O container recebe a
mesma tag (`bridopen-server:v0.1.0`). Os metadados da versão em execução podem
ser consultados em `GET /version`:

```json
{
  "version": "v0.1.0",
  "commit": "a950e68...",
  "build_time": "2026-08-07T20:00:00Z"
}
```

Use incremento de `PATCH` para correções compatíveis, `MINOR` para novas
funcionalidades compatíveis e `MAJOR` para mudanças incompatíveis.

## Licença

Distribuído sob a licença incluída em `LICENSE`.
