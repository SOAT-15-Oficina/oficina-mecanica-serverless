-- Recorte da tabela `users` para desenvolvimento local e para o job de teste do
-- CI. NAO e a fonte da verdade do schema: as migrations vivem em
-- oficina-mecanica-monolith/database/migrations. Manter em sincronia com a
-- definicao de `users` de 20260417000001_create_schema.sql.
CREATE TABLE IF NOT EXISTS "users" (
  "id" uuid PRIMARY KEY,
  "username" varchar(150) NOT NULL,
  "password_hash" varchar(255) NOT NULL,
  "role" varchar(30) NOT NULL,
  "created_at" timestamp NOT NULL,
  "updated_at" timestamp NOT NULL
);

ALTER TABLE "users" DROP CONSTRAINT IF EXISTS users_username_key;
ALTER TABLE "users" ADD CONSTRAINT users_username_key UNIQUE ("username");
