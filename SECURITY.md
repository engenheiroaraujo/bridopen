# Política de Segurança

## Versões suportadas

Apenas a versão mais recente publicada como tag (`vMAJOR.MINOR.PATCH`) e a
branch `main` recebem correções de segurança.

| Versão | Suportada |
|--------|-----------|
| Última tag publicada | ✅ |
| Branch `main` | ✅ |
| Tags anteriores | ❌ |

Quem opera uma instância do Bridopen é responsável por acompanhar as releases e
atualizar sua implantação. A versão em execução pode ser consultada em
`GET /version`.

## Reportando uma vulnerabilidade

**Não crie issues públicas no GitHub para vulnerabilidades de segurança.**

Use o relato privado de vulnerabilidades do GitHub: na aba **Security** do
repositório, clique em **Report a vulnerability**. Isso abre um canal privado
com os mantenedores, sem expor os detalhes até que uma correção esteja
disponível.

Se não conseguir usar esse canal, entre em contato de forma privada pelos meios
indicados no perfil do mantenedor ([@engenheiroaraujo](https://github.com/engenheiroaraujo)).

Ao relatar, inclua:

- Descrição da vulnerabilidade e do impacto potencial
- Passos para reproduzir (rota, payload, configuração de ambiente utilizada)
- Versão do Bridopen afetada (tag, commit ou saída de `GET /version`)
- Configuração relevante, **sem** segredos (por exemplo, se `BRIDOPEN_CLAMAV_ADDR`
  estava ativo, se `BRIDOPEN_COOKIE_SECURE` era `true`)
- Logs ou capturas de tela, removendo e-mails, tokens, senhas e dados de
  usuários reais

O projeto se compromete a **confirmar o recebimento em até 5 dias úteis** e a
informar um prazo para correção ou mitigação **em até 14 dias** para problemas
confirmados. Pedimos que o relato não seja divulgado publicamente antes da
publicação da correção (divulgação coordenada).

Após a correção, a vulnerabilidade será registrada como um GitHub Security
Advisory, com crédito a quem relatou, salvo pedido de anonimato.

## Escopo

O Bridopen é uma aplicação web server-side rendered em Go para anotações, com
autenticação, 2FA, anexos e páginas legais. Consideramos dentro do escopo, entre
outros:

- Quebra de autenticação, sessão, 2FA ou códigos de recuperação
- Bypass de CSRF, rate limit, captcha ou headers de segurança
- Acesso a notas, tags ou anexos de outros usuários (IDOR, quebra de autorização)
- Injeção de SQL, XSS, SSRF ou template injection
- Path traversal, upload de arquivos maliciosos ou bypass do scanner de anexos
- Vazamento de dados pessoais, tokens ou segredos em logs, e-mails ou respostas
- Falhas nos fluxos de exportação e exclusão de conta
- Problemas de supply chain nas dependências Go versionadas em `vendor/` ou na
  imagem Docker construída a partir do repositório

## Fora do escopo

- Textos das páginas legais (privacidade, termos e cookies): são modelos
  genéricos e cabe a quem hospeda revisá-los juridicamente
- Configuração incorreta de instâncias de terceiros (por exemplo, rodar em
  produção sem HTTPS, com `BRIDOPEN_COOKIE_SECURE=false` ou com chaves fracas),
  desde que o README documente o requisito
- Vulnerabilidades em serviços externos usados pela instância (PostgreSQL,
  ClamAV, servidor SMTP, Caddy), que devem ser reportadas aos respectivos
  projetos
- Ataques que exijam acesso físico ou privilegiado ao servidor
- Negação de serviço volumétrica
- Relatórios de scanners automatizados sem prova de exploração

## Boas práticas para quem hospeda

O Bridopen é entregue sem instância oficial. Ao operar sua própria instância,
siga o README e o [`docs/LGPD.md`](docs/LGPD.md), em especial:

- `BRIDOPEN_APP_ENV=production`, `BRIDOPEN_BASE_URL` com HTTPS e
  `BRIDOPEN_COOKIE_SECURE=true`
- `BRIDOPEN_CSRF_KEY` e `BRIDOPEN_MFA_SECRET_KEY` fortes, independentes e fora
  do versionamento
- ClamAV ativo (`BRIDOPEN_CLAMAV_ADDR`) sempre que uploads estiverem habilitados
- Rotação imediata de qualquer segredo que tenha sido exposto
