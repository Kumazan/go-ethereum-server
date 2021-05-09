CREATE TABLE IF NOT EXISTS "transactions" (
    tx_hash varchar(66) PRIMARY KEY,
    block_num integer references blocks(block_num),
    from_addr varchar(66) NOT NULL,
    to_addr varchar(66) NOT NULL,
    nonce bigint NOT NULL,
    data varchar(1024),
    value varchar(30),
    logs json
)