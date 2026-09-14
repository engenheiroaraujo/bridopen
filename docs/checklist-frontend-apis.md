# Checklist de melhoria do frontend com APIs

Este documento propoe uma evolucao incremental para o frontend do Bridopen.
A ideia nao e transformar o projeto em SPA, mas centralizar as chamadas
`fetch()` em pequenos modulos de API no frontend, mantendo o HTML renderizado
pelo Go como base principal da aplicacao.

## Objetivo

- Reduzir `fetch()` espalhado em arquivos de tela.
- Padronizar CSRF, headers, parse de JSON, erros e mensagens.
- Facilitar manutencao quando uma rota mudar.
- Manter formularios e paginas server-rendered funcionando bem.
- Criar uma base simples para novas interacoes dinamicas.

## Situacao atual

- O Bridopen nao possui pasta `frontend/` separada.
- O JavaScript vive em `views/static/js`.
- A maior parte das acoes usa formularios HTML tradicionais.
- Existem chamadas `fetch()` pontuais, principalmente:
  - `views/static/js/account/two-factor-settings.js`
  - `views/static/js/notes/note-delete-modal.js`
- Algumas rotas do Go ja respondem JSON quando a tela pede `Accept: application/json`.

## Resultado esperado

Estrutura sugerida:

```text
views/static/js/
  core/
    http-client.js
    toast.js
    action-lock.js
  api/
    notes-api.js
    two-factor-api.js
    account-api.js
  account/
    two-factor-settings.js
    password-change-modal.js
  notes/
    note-delete-modal.js
```

`core/http-client.js` fica responsavel por:

- Ler CSRF de `<meta name="csrf-token">`.
- Enviar `credentials: "same-origin"`.
- Enviar `Accept: application/json`.
- Enviar `X-CSRF-Token` quando houver token.
- Suportar `application/x-www-form-urlencoded` e JSON.
- Fazer parse seguro de JSON.
- Retornar erro padronizado para a tela.

Os arquivos em `views/static/js/api` ficam responsaveis por conhecer as rotas:

- `twoFactorApi.start(method)`
- `twoFactorApi.verify(method, code)`
- `twoFactorApi.disableStart()`
- `twoFactorApi.disableVerify(code)`
- `twoFactorApi.regenerateRecoveryCodes()`
- `notesApi.delete(url)`

As telas continuam responsaveis apenas por DOM, estado visual, mensagens e foco.

## Checklist

### 1. Inventario das chamadas atuais

- [ ] Mapear todos os `fetch()` em `views/static/js`.
- [ ] Mapear quais formularios retornam HTML e quais podem retornar JSON.
- [ ] Identificar rotas que ja usam `Accept: application/json`.
- [ ] Confirmar quais respostas JSON atuais possuem `ok`, `message`, `redirect`, `status` ou `recovery_codes`.
- [ ] Documentar quais fluxos precisam continuar funcionando sem JavaScript.

### 2. Contrato JSON minimo

- [ ] Definir um formato padrao para sucesso:

```json
{
  "ok": true,
  "message": "Acao concluida.",
  "redirect": "/"
}
```

- [ ] Definir um formato padrao para erro:

```json
{
  "ok": false,
  "message": "Nao foi possivel concluir a acao.",
  "errors": {
    "field": "Mensagem do campo."
  }
}
```

- [ ] Usar status HTTP correto junto do JSON: `200`, `201`, `204`, `400`, `401`, `403`, `404`, `409`, `422`, `429`, `500`.
- [ ] Evitar expor detalhes internos em erros `500`.
- [ ] Nao retornar senha, token, hash, segredo TOTP ou conteudo sensivel em mensagens de erro.

### 3. Criar `core/http-client.js`

- [ ] Criar helper `request(url, options)`.
- [ ] Criar helper `postForm(url, fields)`.
- [ ] Criar helper `deleteJSON(url)`.
- [ ] Centralizar leitura do CSRF.
- [ ] Centralizar tratamento de resposta sem JSON.
- [ ] Centralizar erro de rede.
- [ ] Nao fazer `console.log` de payload sensivel.
- [ ] Manter dependencia zero de bundler.

### 4. Criar modulos em `views/static/js/api`

- [ ] Criar `views/static/js/api/two-factor-api.js`.
- [ ] Mover conhecimento das rotas `/me/twofactor/*` para esse arquivo.
- [ ] Criar `views/static/js/api/notes-api.js`.
- [ ] Mover chamada `DELETE` de nota/anexo para esse arquivo.
- [ ] Criar `views/static/js/api/account-api.js` apenas se houver mais de uma chamada dinamica da area de conta.
- [ ] Exportar funcoes via `window.BridopenApi` ou nomes globais pequenos, ja que o projeto nao usa bundler.

### 5. Refatorar 2FA

- [ ] Remover `postForm()` local de `two-factor-settings.js`.
- [ ] Trocar chamadas diretas por `window.BridopenApi.twoFactor.start(...)`.
- [ ] Manter o estado visual no arquivo da tela.
- [ ] Manter mensagens via `BridopenToast`.
- [ ] Garantir que `action-lock` continua protegendo clique duplo.
- [ ] Testar habilitar 2FA por e-mail.
- [ ] Testar habilitar 2FA por aplicativo autenticador.
- [ ] Testar desabilitar 2FA.
- [ ] Testar regenerar codigos de recuperacao.

### 6. Refatorar exclusao de notas e anexos

- [ ] Remover `fetch(deleteUrl, ...)` direto de `note-delete-modal.js`.
- [ ] Criar `notesApi.delete(deleteUrl)`.
- [ ] Manter `data-note-delete-url` no HTML.
- [ ] Manter `X-Requested-With: XMLHttpRequest` se o backend ainda usa esse header para diferenciar JSON.
- [ ] Manter comportamento de `redirect`, reload e toast.
- [ ] Testar mover nota para lixeira.
- [ ] Testar excluir nota permanentemente.
- [ ] Testar remover anexo.
- [ ] Testar erro de permissao ou item inexistente.

### 7. Padronizar backend para respostas JSON

- [ ] Revisar handlers que respondem JSON manualmente.
- [ ] Criar helper compartilhado de `writeJSON`, se ainda nao houver um ponto unico adequado.
- [ ] Criar helper compartilhado de erro JSON.
- [ ] Padronizar mensagens de erro para requisicoes AJAX.
- [ ] Manter paginas HTML apresentaveis para navegacao comum.
- [ ] Evitar duplicar a mesma estrutura JSON em varios handlers.

### 8. Decidir sobre prefixo `/api`

- [ ] Avaliar se vale criar rotas novas com prefixo `/api`.
- [ ] Se a rota tambem serve formulario HTML, manter a rota atual pode ser melhor.
- [ ] Se a rota e exclusivamente JSON, considerar `/api/...`.
- [ ] Nao misturar mudanca de arquitetura de URL com a primeira refatoracao do frontend.
- [ ] Priorizar primeiro a camada `views/static/js/api`.

### 9. Progressive enhancement

- [ ] Fluxos essenciais devem continuar funcionando sem JavaScript quando fizer sentido.
- [ ] Usar JS para melhorar UX, nao para quebrar o fallback de formulario.
- [ ] Evitar transformar paginas simples em dependentes de JSON sem necessidade.
- [ ] Manter redirects server-side para acoes feitas por formulario.
- [ ] Manter JSON apenas quando a tela pedir via `Accept: application/json` ou header equivalente.

### 10. UX e acessibilidade

- [ ] Padronizar mensagens de sucesso e erro com `BridopenToast`.
- [ ] Padronizar bloqueio de botao com `BridopenActionLock`.
- [ ] Garantir foco apos abrir/fechar modal.
- [ ] Garantir `aria-live` para mensagens importantes.
- [ ] Evitar mudancas visuais sem feedback.
- [ ] Garantir que erro de rede tenha mensagem amigavel.

### 11. Seguranca

- [ ] CSRF deve ser enviado em toda mutacao via JS.
- [ ] Usar sempre `credentials: "same-origin"`.
- [ ] Nao gravar senha, token, codigo 2FA, segredo TOTP ou recovery codes em log.
- [ ] Nao expor stack trace em resposta JSON.
- [ ] Rate limit deve continuar nos fluxos sensiveis.
- [ ] Erros de autenticacao devem ser genericos quando houver risco de enumeracao.

### 12. Testes e verificacao manual

- [ ] Testar fluxos com JavaScript ligado.
- [ ] Testar fallbacks importantes com JavaScript desligado.
- [ ] Testar resposta JSON com CSRF ausente ou invalido.
- [ ] Testar resposta JSON quando a sessao expira.
- [ ] Testar resposta HTML quando a mesma rota e acessada pelo navegador.
- [ ] Adicionar testes Go para handlers que retornam JSON.
- [ ] Considerar testes leves de JS para `http-client.js` se o projeto adotar runner frontend no futuro.

## Ordem recomendada

1. Criar `core/http-client.js`.
2. Criar `api/two-factor-api.js`.
3. Refatorar `two-factor-settings.js`.
4. Criar `api/notes-api.js`.
5. Refatorar `note-delete-modal.js`.
6. Padronizar helpers JSON no Go.
7. Revisar outros modais da conta para repetir o padrao.

## Criterios de pronto

- [ ] Nao existe `fetch()` direto em arquivos de tela quando houver API module correspondente.
- [ ] CSRF e headers ficam centralizados.
- [ ] Cada tela chama funcoes com nome de negocio, nao URLs soltas.
- [ ] Respostas JSON seguem o mesmo formato.
- [ ] Fluxos existentes continuam funcionando.
- [ ] Nenhum dado sensivel aparece em log ou mensagem tecnica.
- [ ] A refatoracao nao exige bundler nem muda o modelo server-rendered do Bridopen.

