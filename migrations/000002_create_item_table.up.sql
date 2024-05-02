CREATE TABLE IF NOT EXISTS "todo"."items" (
    "item_id" BIGSERIAL NOT NULL,
    "list_id" BIGSERIAL NOT NULL,
    "content" text NOT NULL,
    "is_complete" bool NOT NULL,
    PRIMARY KEY ("item_id")
);
