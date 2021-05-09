CREATE TABLE IF NOT EXISTS "blocks" (
    "block_num" INTEGER PRIMARY KEY,
    "block_hash" VARCHAR(66) NOT NULL,
    "block_time" INTEGER NOT NULL,
    "parent_hash" VARCHAR(66) NOT NULL
)