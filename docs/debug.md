# Debug em Tempo Real

Este projeto já possui uma configuração pronta para debug do backend Go no VS Code, usando **Delve**.

A configuração está no arquivo:

```text
.vscode/launch.json
```

```codigo
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Backend",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/http",
      "cwd": "${workspaceFolder}/",
      "envFile": "${workspaceFolder}/.env",
      "console": "integratedTerminal",
      "showLog": true
    }
  ]
}
```

se o Delve não instalado, faça:

```text
    No VS Code:
        Ctrl+Shift+P
        Go: Install/Update Tools
        marque dlv
        instale
```

A opção disponível é:

```text
Debug Backend
```

Ela executa o backend a partir de:

```text
./cmd/http
```

usando as variáveis do arquivo `.env`.

---

## 1. Subir o banco e o MailHog

Antes de iniciar o debug, suba os serviços locais com Docker:

Usando o Makefile:

```powershell
make database
```

---

## 2. Rodar as migrations

Caso as migrations ainda não tenham sido executadas, rode:

```powershell
make migrate-up
```

---

## 3. Conferir o arquivo `.env`

Verifique se o arquivo `.env` está configurado com os valores locais.

Exemplo:

```env
BRIDOPEN_SERVER_PORT=5000
BRIDOPEN_BASE_URL=http://localhost:5000
BRIDOPEN_DB_CONN_URL=postgres://postgres:postgres@localhost:5434/postgres?sslmode=disable
BRIDOPEN_MAIL_HOST=localhost
BRIDOPEN_MAIL_PORT=1025
BRIDOPEN_CSRF_KEY=change-me-with-at-least-32-characters
```

Os pontos mais importantes são:

| Variável          | Descrição                           |
| ----------------- | ----------------------------------- |
| `BRIDOPEN_SERVER_PORT` | Porta onde o backend será executado |
| `BRIDOPEN_BASE_URL`    | URL base da aplicação               |
| `BRIDOPEN_DB_CONN_URL` | String de conexão com o PostgreSQL  |
| `BRIDOPEN_MAIL_HOST`   | Host do servidor de e-mail local    |
| `BRIDOPEN_MAIL_PORT`   | Porta do MailHog                    |
| `BRIDOPEN_CSRF_KEY`    | Chave usada para proteção CSRF      |

---

## 4. Iniciar o debug no VS Code

No VS Code:

1. Instale a extensão oficial **Go**.
2. Abra o menu **Run and Debug**.
3. Selecione a configuração **Debug Backend**.
4. Pressione `F5`.

O VS Code irá iniciar o backend em modo debug usando Delve:

comandos:

Comandos debug go:

| Tecla                 | Ação              | Uso                                                 |
| --------------------- | ----------------- | --------------------------------------------------- |
| **F5**                | Start / Continue  | Inicia o debug ou continua até o próximo breakpoint |
| **F9**                | Toggle Breakpoint | Coloca ou remove breakpoint na linha                |
| **F10**               | Step Over         | Executa a linha atual sem entrar dentro da função   |
| **F11**               | Step Into         | Entra dentro da função chamada                      |
| **Shift + F11**       | Step Out          | Sai da função atual e volta para quem chamou        |
| **Shift + F5**        | Stop              | Para o debug                                        |
| **Ctrl + Shift + F5** | Restart           | Reinicia o debug                                    |

---

## 5. Usar breakpoints

Adicione breakpoints nos pontos que deseja analisar.

Exemplo de arquivo para colocar breakpoint:

```text
internal/handlers/notes/handler/note.go
```

Depois, acesse a aplicação pelo navegador:

```text
http://localhost:5000
```

Quando a execução passar pelo trecho marcado com breakpoint, o VS Code irá pausar o programa.

Durante a pausa, você poderá analisar:

| Recurso       | Finalidade                                    |
| ------------- | --------------------------------------------- |
| Variables     | Ver os valores das variáveis em tempo real    |
| Call Stack    | Ver a sequência de chamadas até o ponto atual |
| Watch         | Monitorar expressões específicas              |
| Debug Console | Executar comandos e inspecionar valores       |
| Step Over     | Avançar para a próxima linha                  |
| Step Into     | Entrar dentro de uma função                   |
| Step Out      | Sair da função atual                          |
| Continue      | Continuar a execução até o próximo breakpoint |

---

## 6. Debug do frontend

O frontend deste projeto utiliza HTML/templates e arquivos JavaScript estáticos.

Alterações em arquivos HTML e estáticos normalmente aparecem após atualizar o navegador:

```text
F5 no navegador
```

Isso acontece porque, em ambiente local, os templates são carregados durante o render.

Já alterações em código Go exigem reiniciar o debug.

Para reiniciar:

```text
Shift + F5
```

Depois:

```text
F5
```

---

## 7. Verificar e-mails locais

O projeto usa MailHog para visualizar e-mails enviados em ambiente local.

Acesse:

```text
http://localhost:8025
```

Lá você poderá verificar e-mails de confirmação, recuperação de senha e outros fluxos que enviam mensagens.

---

## Resumo do fluxo

```text
1. Subir banco e MailHog
2. Rodar migrations
3. Conferir .env
4. Abrir VS Code
5. Selecionar Debug Backend
6. Pressionar F5
7. Colocar breakpoints
8. Acessar http://localhost:5000
9. Analisar a execução em tempo real
```

---

## Observações importantes

* Código Go precisa reiniciar/recompilar após alterações.
* Templates HTML geralmente aparecem no próximo refresh do navegador.
* Arquivos estáticos também podem ser testados com refresh.
* O MailHog fica disponível na porta `8025`.
* O backend roda localmente na porta configurada em `BRIDOPEN_SERVER_PORT`, normalmente `5000`.
