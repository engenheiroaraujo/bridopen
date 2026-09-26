# LGPD e privacidade

Este documento descreve controles tecnicos implementados no Bridopen. Ele nao substitui parecer juridico.

> Aviso: o Bridopen e um projeto opensource. Os textos legais das paginas publicas
> (privacidade, termos e cookies) sao modelos genericos de referencia. Quem operar
> uma instancia e o controlador dos dados e deve revisar esses textos com apoio
> juridico antes de publicar.

## Dados tratados

- E-mail do usuario.
- Senha protegida por hash bcrypt.
- Tokens operacionais de confirmacao e redefinicao de senha, armazenados apenas como hash.
- Conteudo das anotacoes criadas pelo usuario.
- Cookies essenciais de sessao e CSRF.
- Preferencia de tema salva localmente no navegador.

## Controles implementados

- `.env` fora do versionamento e fora da imagem final do Docker.
- URL publica configurada por `BRIDOPEN_BASE_URL`, sem depender do header `Host` para links de e-mail.
- Cookies de sessao HTTP-only, SameSite=Lax e Secure obrigatorio em producao.
- CSRF com cookie HTTP-only, SameSite=Lax, origins configuraveis e erro generico para o usuario.
- Tokens com geracao criptografica, hash SHA-256 no banco, finalidade e expiracao.
- Redefinicao de senha com resposta generica para reduzir enumeracao de contas.
- Logs sem e-mail, senha, token, hash ou conteudo de notas.
- Auditoria persistente no schema `auditoria`, com tabela `audit_events`.
- Exportacao e exclusao de conta na area autenticada.
- Paginas publicas de privacidade e cookies.

## Variaveis principais

- `BRIDOPEN_APP_ENV`: `development` ou `production`.
- `BRIDOPEN_BASE_URL`: URL publica absoluta da aplicacao.
- `BRIDOPEN_COOKIE_SECURE`: deve ser `true` em producao.
- `BRIDOPEN_TRUSTED_ORIGINS`: lista separada por virgula com dominios aceitos pelo CSRF.
- `BRIDOPEN_CSRF_KEY`: segredo forte com pelo menos 32 caracteres em producao.

## Pontos operacionais

- Em producao, use HTTPS no proxy reverso.
- Rode migrations antes de iniciar a aplicacao.
- Remova historico de segredos que ja tenham sido commitados e rotacione as credenciais expostas.
- A auditoria fica em `auditoria.audit_events` e usa somente ids internos, acao, entidade, resultado e data do evento.
