# agroStock_API

API em Go para controle de estoque de insumos agrícolas (SQL Server).

## Como rodar

1. Crie as tabelas com [sql/schema.sql](sql/schema.sql).
2. Copie `.env.example` para `.env` e ajuste `DATABASE_URL`.
3. Crie um usuário (a senha é pedida no terminal e salva como hash bcrypt):

   ```sh
   go run ./cmd/criar-usuario -login admin
   ```

4. Suba a API:

   ```sh
   go run ./cmd/api
   ```

5. Testes: `go test ./...`

## Autenticação

`POST /sessao` com `{"login","senha"}` devolve um cookie `agrostock_sessao`
(HttpOnly, SameSite=Lax). Todas as rotas abaixo, exceto `/health`,
`POST /sessao`, `POST /usuarios` e `POST /senha/*`, respondem **401** sem esse cookie.

### Recuperação de senha

A API ainda **não envia e-mail**. Em desenvolvimento, defina
`RESET_TOKEN_NA_RESPOSTA=true` no `.env`: o token volta na resposta de
`POST /senha/esqueci` (campo `tokenRedefinicao`) e o frontend o mostra. Em
produção deixe `false` — senão qualquer pessoa troca a senha de qualquer conta
sabendo o e-mail. Só o hash SHA-256 do token fica gravado no banco.

Bancos criados antes do e-mail precisam da migração
[sql/migracao-001-email-recuperacao-senha.sql](sql/migracao-001-email-recuperacao-senha.sql).

No frontend, envie o cookie em toda chamada (`fetch(url, { credentials: "include" })`)
e use `Content-Type: application/json` nos POSTs.

## Rotas

| Método | Rota | Descrição |
|---|---|---|
| GET | `/health` | Verifica a conexão com o banco |
| POST | `/usuarios` | Cadastro público `{login, email, senha}` (login sem espaços nem `@`, até 50; senha de 8 a 72 bytes). Não abre sessão: depois, faça o login. 409 se o login ou o e-mail existir |
| POST | `/sessao` | Login. `login` aceita o usuário ou o e-mail |
| POST | `/senha/esqueci` | `{email}`. Gera um token de redefinição válido por 30 min. Resposta sempre igual, exista ou não a conta |
| POST | `/senha/redefinir` | `{token, novaSenha}`. Troca a senha e invalida o token. 400 se o token for inválido ou expirado |
| GET | `/sessao` | Usuário logado |
| DELETE | `/sessao` | Logout |
| POST | `/insumos` | `{nome, categoria, unidade, minimo}`, com `minimo >= 0` |
| GET | `/insumos` | Lista insumos |
| POST | `/lotes` | `{insumoId, codigo, validade: "AAAA-MM-DD"}`; o lote nasce com saldo 0 |
| GET | `/lotes?insumoId=` | Lista lotes |
| POST | `/movimentacoes` | `{loteId, tipo: "E"\|"S", quantidade, motivo?, chaveRequisicao}` |
| GET | `/movimentacoes?insumoId=&loteId=&tipo=&de=&ate=&limite=` | Histórico (mais recentes primeiro, padrão 100, máx. 500), com `saldoLote` (saldo do lote logo após cada operação) |
| GET | `/saldos?insumoId=` | Saldo consolidado por insumo, com lotes e `abaixoMinimo` |
| GET | `/alertas` | Lotes com saldo e validade em ≤ 7 dias (`vermelho`, inclui vencidos) ou de 8 a 30 dias (`amarelo`) |

### Movimentações

- Tudo roda em **uma transação**: trava o lote, confere o saldo, atualiza e grava
  o histórico. Se algo falhar, nada é gravado.
- Saída sem saldo suficiente devolve **409** com `saldoDisponivel`.
- **Duplo clique:** gere uma `chaveRequisicao` nova (ex: `crypto.randomUUID()`)
  para cada operação e reenvie a *mesma* chave em caso de retry. Uma chave
  repetida devolve **200** com a movimentação original (cabeçalho
  `Idempotent-Replayed: true`) sem retirar de novo. Uma chave já usada com dados
  diferentes devolve **409**.

### Erros

Sempre no formato `{"erro": "mensagem"}`: 400 (requisição malformada), 401, 404,
409 (duplicidade/saldo), 415 (Content-Type), 422 (validação), 500.
