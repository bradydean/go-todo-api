CREATE SCHEMA IF NOT EXISTS "todo";

CREATE TABLE IF NOT EXISTS "todo"."lists" (
    "list_id" BIGSERIAL NOT NULL,
    "user_id" TEXT NOT NULL,
    "title" text NOT NULL,
    "description" text NOT NULL,
    PRIMARY KEY ("list_id")
);

CREATE INDEX IF NOT EXISTS "lists_user_id_index" ON "todo"."lists" ("user_id");
