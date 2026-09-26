# Identidade visual do Bridopen

Atualizado: 2026-09-26

## Decisão aprovada

O usuário aprovou a [prévia inspirada no Bear](prototipos/notas-bear.html) e autorizou aplicar essa direção a todas as telas. Ela substitui a proposta editorial anterior, rejeitada após avaliação visual. O protótipo permanece como referência, com dados fictícios; a aplicação usa os dados reais das rotas existentes.

## Sistema visual

- Navegação cinza `#f2f2f0`, lista `#fafaf9`, documento branco e texto `#30302e`.
- Seleção suave `#f4e9e5`; botões e links em terracota `#a34f39`.
- Tema escuro com superfícies neutras `#232322`, `#292928`, `#30302e` e destaque claro.
- Tipografia do sistema, títulos compactos, bordas discretas e controles de 5–6 px de raio.
- Marca e favicon Bridopen preservados conforme a prévia aprovada.
- Autenticação em coluna única. Conta, edição, modais, páginas públicas e e-mails seguem a mesma direção.

## Lista e leitura

`views/templates/pages/home.html` apresenta lista compacta e painel de leitura. Cada nota inclui um HTML `<template>` com título e conteúdo escapados pelo Go. `views/static/js/notes/note-reading-pane.js` clona o template selecionado, marca o item ativo e usa o ciclo de vida existente para remover listeners na navegação parcial.

No desktop, a primeira nota é selecionada inicialmente. No celular, a lista aparece primeiro; selecionar abre a leitura, e Voltar devolve o foco ao item. Links preservam suas URLs: abrir em outra aba e navegação sem JavaScript seguem para a página completa. Edição e anexos utilizam as rotas existentes; não há escrita local ou novo endpoint. Busca, ordenação, etiquetas, cor e fixação continuam processadas pelo servidor. Cores e etiquetas ficam em um expansor, aberto quando existem filtros ativos.

As cores das notas continuam em `--note-accent`: faixa lateral na lista inicial, faixa superior nos demais cartões. Segurança, dados, validações, captcha, CSRF e ações existentes permanecem nos handlers originais.

## Arquivos principais

- `views/static/css/style.css` e `views/static/js/core/tailwind-config.js`: tokens e apresentação.
- `views/templates/base*.html`, `partials/app-sidebar.html`, `partials/auth-layout.html`: shell e autenticação.
- `views/templates/mails/`: cores e tipografia dos e-mails.
- `tests/e2e/note-reading-pane.spec.js`: seleção, navegação, leitura no celular e conteúdo escapado.

## Operação e verificação

Os assets são incorporados por `go:embed`: recompilar/reiniciar o servidor para carregar alterações. O preview `cmd/e2e-preview` permite revisar templates reais usando fixtures sem banco de dados.

Verificar com `npm run test:frontend`, `npm run test:e2e` e `go test ./views ./internal/platform/render ./internal/handlers/users/handler ./internal/handlers/notes/handler`.

O repositório não possui `docs/segundo-cerebro/`; esta nota registra a decisão na documentação existente.
