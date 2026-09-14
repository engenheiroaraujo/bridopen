# Checklist de versionamento e releases

> Nota: este repositório publico nao inclui workflow de deploy. As etapas de
> deploy abaixo descrevem um pipeline self-hosted e devem ser adaptadas a
> infraestrutura de quem opera a instancia.


Este documento descreve como versionar e publicar o Bridopen com segurança.
A tag Git no formato `vMAJOR.MINOR.PATCH` é a fonte oficial da versão. O envio
da tag ao repositório dispara o workflow de testes e deploy de produção.

## Como interpretar uma versão

Considere a versão `v1.4.2`:

- `1` é o **MAJOR**: aumente quando houver mudança incompatível importante.
- `4` é o **MINOR**: aumente ao adicionar funcionalidade compatível.
- `2` é o **PATCH**: aumente ao corrigir um problema sem quebrar compatibilidade.
- O prefixo `v` identifica que o nome é uma versão Git do projeto.

Exemplos:

- `v0.1.0` para a primeira versão utilizável ainda em desenvolvimento.
- `v0.2.0` depois de adicionar uma nova funcionalidade.
- `v0.2.1` depois de corrigir um erro da `v0.2.0`.
- `v1.0.0` quando o produto e seus contratos forem considerados estáveis.
- `v2.0.0` quando uma mudança incompatível for necessária após a `v1`.

Enquanto o projeto estiver na série `v0.x.x`, mudanças relevantes podem ocorrer
com maior frequência. Ainda assim, use `MINOR` para mudanças funcionais e
`PATCH` para correções, mantendo o histórico previsível.

## 1. Escolher o próximo número

- [ ] Verificar a versão publicada mais recente:

  ```powershell
  git fetch --tags
  git tag --sort=-version:refname
  ```

- [ ] Classificar as mudanças desde a última versão.
- [ ] Incrementar apenas uma parte da versão:
  - Correção compatível: `v0.2.0` → `v0.2.1`.
  - Nova funcionalidade compatível: `v0.2.1` → `v0.3.0`.
  - Mudança incompatível: `v1.4.2` → `v2.0.0`.
- [ ] Confirmar que a versão ainda não existe localmente nem no remoto.
- [ ] Evitar reutilizar um número que já tenha sido publicado.

## 2. Preparar a release

- [ ] Trabalhar em uma branch e concluir as alterações necessárias.
- [ ] Revisar arquivos modificados:

  ```powershell
  git status
  git diff
  ```

- [ ] Executar a suíte completa de testes:

  ```powershell
  go test ./...
  ```

- [ ] Validar a configuração de produção:

  ```powershell
  docker compose -f docker-compose.yml config --quiet
  ```

- [ ] Verificar migrations novas e sua compatibilidade com a versão anterior.
- [ ] Confirmar que novos parâmetros de ambiente estão documentados no
  `.env.example` e configurados no ambiente de produção.
- [ ] Registrar resumidamente as funcionalidades, correções, migrations e
  eventuais ações manuais da release.
- [ ] Fazer commit de todas as alterações que devem integrar a versão.

Uma tag aponta para um commit exato. Portanto, arquivos sem commit não entram
na release, mesmo que estejam presentes no computador de quem criou a tag.

## 3. Atualizar a branch principal

- [ ] Enviar a branch e concluir sua integração na `main`.
- [ ] Atualizar a cópia local da `main`:

  ```powershell
  git switch main
  git pull --ff-only
  ```

- [ ] Confirmar que não há alterações locais pendentes:

  ```powershell
  git status
  ```

- [ ] Executar novamente os testes se a `main` mudou durante a integração.
- [ ] Conferir o commit que receberá a tag:

  ```powershell
  git log -1 --oneline
  ```

## 4. Criar e publicar a tag

- [ ] Criar uma tag **anotada**, substituindo `v0.1.0` pela nova versão:

  ```powershell
  git tag -a v0.1.0 -m "Release v0.1.0"
  ```

- [ ] Conferir para qual commit a tag aponta:

  ```powershell
  git show v0.1.0 --no-patch
  ```

- [ ] Publicar somente a tag desejada:

  ```powershell
  git push origin v0.1.0
  ```

O padrão `vMAJOR.MINOR.PATCH` ativa o workflow de deploy. Publicar a tag é uma
ação de produção; faça isso apenas depois das validações anteriores.

## 5. Acompanhar o deploy

- [ ] Abrir a execução correspondente em **GitHub Actions**.
- [ ] Confirmar que os testes passaram.
- [ ] Confirmar que a sincronização e o build no servidor terminaram sem erro.
- [ ] Verificar se os containers estão saudáveis e se não reiniciam em loop.
- [ ] Consultar os logs do servidor quando necessário:

  ```powershell
  make prod-logs
  ```

- [ ] Confirmar os metadados da aplicação implantada:

  ```text
  GET https://seu-dominio.com.br/version
  ```

  A resposta esperada tem este formato:

  ```json
  {
    "version": "v0.1.0",
    "commit": "hash-do-commit",
    "build_time": "2026-08-11T12:00:00Z"
  }
  ```

- [ ] Verificar se `version` corresponde à tag publicada.
- [ ] Verificar se `commit` corresponde ao commit exibido por `git show`.
- [ ] Fazer um teste rápido das funções essenciais da aplicação.
- [ ] Registrar o resultado do deploy e qualquer ocorrência relevante.

## 6. Builds locais

Ao executar `go run` ou `go build` sem parâmetros adicionais, o endpoint mostra
valores de desenvolvimento:

```json
{
  "version": "dev",
  "commit": "unknown",
  "build_time": "unknown"
}
```

Isso é esperado e evita manter a versão duplicada em um arquivo manual. No
deploy, o workflow define `BRIDOPEN_VERSION`, `BRIDOPEN_COMMIT` e `BRIDOPEN_BUILD_TIME`; o
Docker os incorpora ao binário durante a compilação.

Para simular localmente uma imagem versionada:

```powershell
$env:BRIDOPEN_VERSION = "v0.1.0"
$env:BRIDOPEN_COMMIT = git rev-parse HEAD
$env:BRIDOPEN_BUILD_TIME = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
docker compose build server
```

## 7. Correções após uma release

Se um problema for encontrado depois do deploy:

- [ ] Avaliar o impacto e decidir se é necessário corrigir imediatamente.
- [ ] Corrigir o código em uma nova branch e executar os testes.
- [ ] Integrar a correção na `main`.
- [ ] Criar uma **nova** versão `PATCH`.
- [ ] Publicar a nova tag e repetir a validação do deploy.

Exemplo: se o problema está na `v0.3.0`, publique a correção como `v0.3.1`.
Não mova a tag `v0.3.0` para outro commit, pois isso torna o histórico ambíguo.

## 8. Tag criada ou publicada por engano

Se a tag ainda existe somente localmente, remova-a e crie a correta:

```powershell
git tag -d v0.1.0
```

Se a tag já foi enviada ao remoto, prefira interromper o deploy e publicar um
novo número depois da correção. Excluir uma tag remota pode afetar outras
pessoas e automações; faça isso somente após alinhamento com a equipe.

Nunca altere silenciosamente uma tag que já gerou um artefato ou deploy.

## Checklist resumido

- [ ] Número escolhido conforme `MAJOR.MINOR.PATCH`.
- [ ] Código revisado e commitado.
- [ ] Testes Go aprovados.
- [ ] Docker Compose validado.
- [ ] Migrations e variáveis de ambiente revisadas.
- [ ] `main` atualizada e sem alterações locais.
- [ ] Tag anotada criada no commit correto.
- [ ] Tag publicada no remoto.
- [ ] GitHub Actions concluído com sucesso.
- [ ] `/version` corresponde à tag e ao commit publicados.
- [ ] Teste rápido de produção concluído.
- [ ] Resultado da release registrado.
