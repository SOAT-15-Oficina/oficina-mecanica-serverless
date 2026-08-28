# oficina-mecanica-serverless

Função de autenticação da plataforma. Uma Lambda em Go que serve
`POST /auth/login` e `POST /auth/register` atrás do API Gateway, e emite o JWT
que o [`oficina-mecanica-monolith`](https://github.com/SOAT-15-Oficina/oficina-mecanica-monolith)
valida.

> **Visão de arquitetura do sistema completo** vive em
> [`oficina-mecanica-infrastructure`](https://github.com/SOAT-15-Oficina/oficina-mecanica-infrastructure).
> Este README cobre apenas este repositório.

## Fronteiras

Esta função **lê e escreve na tabela `users` do mesmo RDS** que o monolito usa.

Isso é deliberado: `users.id` é alvo de chave estrangeira em
`work_orders.opened_by_user_id`, `work_orders.assigned_technician_id` e
`work_order_status_history.changed_by_user_id`. Dar um banco próprio a esta
função quebraria três FKs e o fluxo de abertura de OS.

| | |
|---|---|
| **Dono do schema** | `oficina-mecanica-monolith` (`database/migrations`) |
| **Migrations rodadas aqui** | nenhuma — esta função depende do schema já existir |
| **CRUD administrativo `/users`** | fica no monolito |
| **Criação da função, VPC, IAM, API Gateway** | `oficina-mecanica-infrastructure` |

`tests/bootstrap.sql` é um **recorte** de `users` para uso local e no CI, não a
fonte da verdade. Se a definição da tabela mudar no monolito, atualize-o.

## Contrato do token

Uma cópia de `AppClaims`/`GenerateToken`/`ParseToken` vive em `internal/auth/`
aqui **e** no monolito. São ~40 linhas — não valem um módulo Go compartilhado
(que exigiria `GOPRIVATE` e deploy key nos dois CIs) nem um quinto repositório.

O que impede as cópias de divergirem é o fixture `internal/auth/testdata/token.golden`:
byte a byte o mesmo arquivo nos dois repositórios, com expiração fixa em 2099.
Cada repo tem um teste que o parseia. Renomear um claim ou trocar o algoritmo de
assinatura quebra o build dos dois, em vez de virar 401 em produção.

```bash
go run ./tools/gentoken > internal/auth/testdata/token.golden
cp internal/auth/testdata/token.golden \
   ../oficina-mecanica-monolith/internal/auth/testdata/token.golden
```

Regenere e copie **no mesmo PR**.

## Estrutura

```
cmd/lambda/          bootstrap: config, pool, handler, lambda.Start
internal/
  auth/              AppClaims + Generate/ParseToken + fixture de contrato
  config/            env + Secrets Manager (com fallback local)
  domain/            User e UserRole
  handler/           roteamento por RouteKey, tradução de erro → HTTP
  repository/        adaptador pgx da tabela users
  service/           argon2 (hash/verify) + Register/Login
tools/gentoken/      gerador do fixture de contrato
docs/openapi.yaml    contrato — apenas /auth/*
tests/bootstrap.sql  recorte de `users` para local e CI
```

## Rodando local

```bash
docker compose up --build
```

Sobe a função no **Lambda Runtime Interface Emulator** com um Postgres ao lado.
Sem `DATABASE_SECRET_ID`/`JWT_SECRET_ID` no ambiente, o config lê direto das
variáveis e **não chama o Secrets Manager** — não há AWS envolvida.

```bash
curl -s localhost:9000/2015-03-31/functions/function/invocations -d '{
  "routeKey": "POST /auth/register",
  "body": "{\"username\":\"admin\",\"password\":\"admin123\",\"role\":\"admin\"}"
}'
```

Para o fluxo completo (front + Lambda + monolito atrás de um nginx que replica o
roteamento do CloudFront), use `docker-compose.local.yml` no repositório de
infraestrutura.

## Testes

```bash
go test ./... -short                      # unitários, sem banco
TEST_DATABASE_URL=postgres://... go test ./...   # inclui integração
go test ./... -coverprofile=coverage.out
```

Os testes de integração de `internal/repository` pulam sozinhos quando
`TEST_DATABASE_URL` não está definida.

## Qualidade

```bash
docker compose -f docker-compose.sonar.yml up -d sonarqube   # :9001
go test ./... -coverprofile=coverage.out
docker compose -f docker-compose.sonar.yml run --rm sonar-scanner
```

O CI sobe uma instância equivalente como service container efêmero e o Quality
Gate **bloqueia o merge**. Sem histórico entre execuções, o gate é avaliado sobre
o código todo, não sobre *new code*.

## Empacotamento e deploy

Produção é um **zip com um binário `bootstrap`** em `provided.al2023`, compilado
para **arm64** (Graviton). Sem container image: o binário tem poucos MB e o cold
start fica em ~50–150ms, mais o ENI da VPC na primeira invocação.

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o dist/bootstrap ./cmd/lambda
(cd dist && zip -X function.zip bootstrap)
```

O CI faz isso e publica com `aws lambda update-function-code`, resolvendo o nome
da função no SSM:

| Parâmetro | Uso |
|---|---|
| `/oficina-mecanica/prod/auth_lambda_name` | alvo do `update-function-code` |

A função é **criada pelo Terraform** com um zip placeholder e
`lifecycle.ignore_changes` no código: o **artefato** é propriedade deste
pipeline; **VPC, memória, timeout, role e variáveis de ambiente** são
propriedade do repositório de infraestrutura.

### Secrets e variables necessários

| Nome | Tipo | Conteúdo |
|---|---|---|
| `AWS_DEPLOY_ROLE_ARN` | secret | role assumida por OIDC (`lambda:UpdateFunctionCode` + SSM) |
| `CI_DATABASE_PASSWORD` | secret | senha do Postgres do job de teste |
| `AWS_REGION` | variable | `sa-east-1` |

## Rede e segredos em produção

A função roda **dentro da VPC, em subnets privadas**, com SG próprio liberado no
SG do RDS na 5432 — o banco nunca é exposto à internet. Credencial do RDS e
chave JWT vêm do **Secrets Manager**, lidos uma vez por container (não por
invocação): em container reutilizado já estão em memória.

O pool tem teto baixo de conexões por container, porque concorrência de Lambda
multiplica pools. Se o volume de login crescer, o próximo passo é **RDS Proxy**,
não aumentar esse número.

## Riscos conhecidos

**`POST /auth/register` é público e aceita o campo `role`.** Qualquer chamador
pode criar um usuário `admin`. Esse é o comportamento herdado do monolito,
mantido de forma deliberada para que o split fosse estritamente estrutural — e
agora exposto numa URL pública na internet.

Corrigir significa exigir um token de admin antes de registrar (a função já tem
`ParseToken`) e semear o primeiro admin por uma migration de bootstrap. Está
registrado no contrato OpenAPI e aqui para não ser esquecido.
