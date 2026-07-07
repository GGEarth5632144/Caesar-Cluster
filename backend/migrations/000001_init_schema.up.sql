CREATE TABLE users (
    id          SERIAL PRIMARY KEY,
    student_id  VARCHAR(20) UNIQUE NOT NULL,
    name        VARCHAR(100) NOT NULL,
    role        VARCHAR(10) NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    password    VARCHAR(255) NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE nodes (
    id           SERIAL PRIMARY KEY,
    hostname     VARCHAR(50) UNIQUE NOT NULL,
    total_cores  INT NOT NULL,
    total_ram_mb INT NOT NULL,
    status       VARCHAR(20) NOT NULL DEFAULT 'online'
);

CREATE TABLE vms (
    id         SERIAL PRIMARY KEY,
    owner_id   INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    node_id    INT NOT NULL REFERENCES nodes(id),
    name       VARCHAR(50) NOT NULL,
    cpu_cores  INT NOT NULL CHECK (cpu_cores > 0),
    ram_mb     INT NOT NULL CHECK (ram_mb > 0),
    config     JSONB,
    status     VARCHAR(20) NOT NULL DEFAULT 'creating',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vms_owner ON vms(owner_id);
CREATE INDEX idx_vms_node  ON vms(node_id);