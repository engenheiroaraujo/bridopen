# Contribuindo com o Bridopen

Obrigado por dedicar seu tempo a contribuir! 🎉
Este guia ajuda você a começar rapidamente.

---

## Sumário

- [Primeiros passos](#primeiros-passos)
- [Estrutura do projeto](#estrutura-do-projeto)
- [Formas de contribuir](#formas-de-contribuir)
- [Fluxo de desenvolvimento](#fluxo-de-desenvolvimento)
- [Padrões de código](#padrões-de-código)
- [Formato das mensagens de commit](#formato-das-mensagens-de-commit)
- [Diretrizes para pull requests](#diretrizes-para-pull-requests)
- [Reportando bugs](#reportando-bugs)
- [Reportando vulnerabilidades](#reportando-vulnerabilidades)
- [Código de Conduta](#código-de-conduta)

---

## Primeiros passos

### Pré-requisitos

- **Go** 1.27+
- **Docker** e Docker Compose
- **GNU Make**
- **golang-migrate** (CLI `migrate`)
- **Node.js** 22+ e **npm** (apenas para os testes de frontend)
- **Git**

### Fork e clone

```bash
# 1. Faça um fork do repositório no GitHub e clone o seu fork
git clone https://github.com/SEU_USUARIO/bridopen.git
cd bridopen

# 2. Adicione o repositório original como upstream
git remote add upstream https://github.com/engenheiroaraujo/bridopen.git

# 3. Suba PostgreSQL e MailHog locais
make database

# 4. Aplique as migrations
make migrate-up

# 5. Crie o .env a partir do exemplo e inicie o servidor
cp .env.example .env
make server
```

Para usar outro banco nas migrations, defina `BRIDOPEN_DB_CONN_URL` no ambiente antes de rodar o `make migrate-up`.

Acesse [http://localhost:5001](http://localhost:5001). O MailHog fica em
[http://localhost:8025](http://localhost:8025) para inspecionar os e-mails
enviados em desenvolvimento.

---

## Estrutura do projeto

```
bridopen/
├── cmd/http/              # Composition root: config, DB, sessão, rotas
├── db/migrations/         # Migrations SQL versionadas (golang-migrate)
├── internal/
│   ├── handlers/
│   │   ├── legal/         # Páginas públicas: privacidade, termos, cookies
│   │   ├── notes/         # Módulo de notas: dto, handler, model, repositories, service
│   │   └── users/         # Módulo de usuários: auth, conta, 2FA
│   └── platform/          # Infraestrutura transversal: config, errors, security, render...
├── views/
│   ├── static/            # CSS, JS e imagens
│   └── templates/         # Templates html/template (pages, partials, mails)
├── tests/                 # Testes de frontend (Node) e E2E (Playwright)
└── docs/                  # Documentação auxiliar
```

O projeto é um **monolito modular em camadas**. Cada módulo de negócio segue a
separação abaixo, e contribuições devem respeitá-la:

| Camada | Responsabilidade |
|--------|------------------|
| `handler` | HTTP, sessão, request/response, render e JSON. Sem regra de negócio. |
| `service` | Casos de uso, validações de fluxo e orquestração. |
| `model` | Entidades, estados, constantes e regras puras de domínio. |
| `repositories` | SQL, scans, transações e mapeamento de persistência. |
| `dto` | Page models e DTOs entregues aos templates. |
| `internal/platform` | Infraestrutura e preocupações transversais. |

> **Importante:** templates recebem DTOs/page models, nunca regras de negócio.
> Não coloque SQL fora de `repositories` nem lógica de domínio dentro de handlers.

---

## Formas de contribuir

### 🐛 Correção de bugs
Consulte a [aba de Issues](https://github.com/engenheiroaraujo/bridopen/issues)
e procure itens com a label `bug`. Comente na issue antes de começar para
evitarmos trabalho duplicado.

### ✨ Novas funcionalidades
Abra uma issue descrevendo o problema que a funcionalidade resolve **antes** de
implementar. Funcionalidades grandes sem discussão prévia podem não ser aceitas,
mesmo que bem implementadas.

### 🔒 Segurança e privacidade
Melhorias em CSRF, headers, rate limit, 2FA, upload de anexos e controles de
LGPD são muito bem-vindas. Leia [`docs/LGPD.md`](docs/LGPD.md) antes. Para
**vulnerabilidades**, siga a [Política de Segurança](SECURITY.md) e não abra
issue pública.

### 🎨 Frontend
O projeto não usa SPA nem bundler. JavaScript fica em `views/static/js/`,
separado por responsabilidade (`core/`, `notes/`, `account/`). Feedback de ações
comuns usa o toast global; modais ficam para confirmações importantes ou ações
destrutivas. Consulte [`docs/checklist-frontend-apis.md`](docs/checklist-frontend-apis.md).

### 🗄️ Migrations
Crie migrations com `make migrate-create name=descricao_curta`. Toda migration
`up` precisa de um `down` funcional. Nunca edite uma migration já aplicada em
`main`; crie uma nova.

### 🌍 Traduções e textos
Correções de textos da interface, e-mails e páginas legais são bem-vindas.
Lembre que os textos legais (`privacy`, `terms`, `cookies`) são modelos
genéricos e não constituem parecer jurídico.

### 📝 Documentação
Corrija erros, esclareça trechos confusos ou adicione exemplos no `README.md` e
em `docs/`.

---

## Fluxo de desenvolvimento

```bash
# 1. Atualize sua main com o upstream
git switch main
git pull upstream main

# 2. Crie uma branch a partir da main
git switch -c feat/nome-da-funcionalidade

# 3. Faça suas alterações respeitando as camadas do módulo

# 4. Formate, analise e rode os testes Go
gofmt -l .            # não deve listar nenhum arquivo
go vet ./...
make test

# 5. Se alterou templates, CSS ou JS, rode também os testes de frontend
make test-e2e-install # apenas na primeira vez
make test-frontend
make test-e2e

# 6. Envie a branch para o seu fork e abra o pull request
git push -u origin feat/nome-da-funcionalidade
```

`make test-all` executa Go, Node e Playwright em sequência. O CI roda
exatamente esses passos em cada pull request; ele precisa passar antes da
revisão.

Prefixos sugeridos para branches: `feat/`, `fix/`, `docs/`, `refactor/`,
`test/` e `chore/`.

---

## Padrões de código

- **Go**: código formatado com `gofmt` e sem avisos do `go vet`. Identificadores,
  pacotes, erros internos e logs em inglês; a linguagem do produto (português)
  fica reservada a substantivos de domínio e textos exibidos ao usuário.
- **Erros**: use os erros tipados de `internal/platform/errors`. Não exponha
  detalhes internos ao usuário final.
- **Logs**: nunca registre e-mail, senha, token, hash ou conteúdo de notas.
- **Segurança**: toda rota que altera estado passa por CSRF; entradas são
  validadas na borda com `internal/platform/validations`; uploads respeitam o
  fluxo de scanner descrito em [`docs/fluxo-notas-anexos.md`](docs/fluxo-notas-anexos.md).
- **Dependências**: o diretório `vendor/` é versionado. Ao adicionar ou
  atualizar dependências, rode `go mod tidy && go mod vendor` e inclua o
  resultado no mesmo commit.
- **Testes**: novas regras de negócio precisam de testes em Go. Alterações em
  templates base devem manter `views/base_scripts_test.go` passando.
- **Editor**: respeite o `.editorconfig` (UTF-8, quebra de linha final, sem
  espaços à direita).

---

## Formato das mensagens de commit

Seguimos o [Conventional Commits](https://www.conventionalcommits.org/pt-br/):

```
<tipo>(<escopo opcional>): <descrição curta no imperativo>

Tipos:
  feat     → nova funcionalidade
  fix      → correção de bug
  docs     → apenas documentação
  refactor → mudança de código sem nova funcionalidade nem correção
  test     → adição ou correção de testes
  chore    → build, dependências, CI e tarefas de manutenção
  security → correção ou endurecimento de segurança
```

**Exemplos:**
```
feat(notes): permitir filtrar notas arquivadas por tag
fix(users): corrigir expiração do token de recuperação de senha
docs: documentar variável BRIDOPEN_CLAMAV_ADDR
security(attachments): rejeitar upload quando o scanner estiver indisponível
chore: atualizar pgx para v5.8
```

Mensagens podem ser escritas em português ou inglês, mas mantenha o mesmo idioma
dentro de um mesmo pull request.

---

## Diretrizes para pull requests

1. **Um PR por mudança** — mantenha os PRs pequenos e focados.
2. **Referencie a issue relacionada** — use `Closes #123` na descrição.
3. **Descreva o que mudou e por quê** — inclua como testar e, para mudanças
   visuais, capturas de tela antes/depois.
4. **CI verde** — `go vet`, testes Go, testes de frontend e Playwright precisam
   passar.
5. **Nunca faça push direto na `main`** — sempre use uma branch.
6. **Migrations e `vendor/`** — se houver, destaque-os na descrição do PR.
7. **Aguarde a revisão** — um mantenedor revisará em alguns dias. Responda aos
   comentários com novos commits em vez de force-push, para facilitar o
   acompanhamento.

Mudanças que quebrem compatibilidade (variáveis de ambiente, rotas, schema do
banco) devem ser destacadas na descrição, pois afetam o incremento de versão
descrito em [`docs/checklist-versionamento.md`](docs/checklist-versionamento.md).

---

## Reportando bugs

Abra uma [issue](https://github.com/engenheiroaraujo/bridopen/issues/new) e
inclua:

- Versão do Bridopen (saída de `GET /version` ou a tag/commit usado)
- Sistema operacional e forma de execução (Docker Compose, `make server`, etc.)
- Versão do Go e do PostgreSQL, quando relevante
- Passos para reproduzir
- Comportamento esperado e comportamento observado
- Logs relevantes ou capturas de tela, **sem** e-mails, tokens, senhas ou
  conteúdo de notas de terceiros

---

## Reportando vulnerabilidades

**Não abra issue pública para vulnerabilidades de segurança.** Siga o processo
descrito em [`SECURITY.md`](SECURITY.md).

---

## Código de Conduta

Este projeto adota o [Código de Conduta](CODE_OF_CONDUCT.md) baseado no
Contributor Covenant. Ao participar, você concorda em respeitá-lo.

Seja gentil, construtivo e respeitoso. Assédio, spam e contribuições de baixo
esforço (por exemplo, PRs gerados automaticamente sem revisão) serão fechados
sem análise.

---

## Dúvidas?

Abra uma [Discussion](https://github.com/engenheiroaraujo/bridopen/discussions)
ou consulte primeiro o [README](README.md) e a pasta [`docs/`](docs/).

Boas contribuições! 🚀
