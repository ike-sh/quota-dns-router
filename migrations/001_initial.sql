CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    switch_cooldown_seconds INTEGER NOT NULL DEFAULT 600,
    current_node_id TEXT,
    last_switch_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cloudflare_configs (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL UNIQUE,
    api_token TEXT NOT NULL,
    zone_name TEXT NOT NULL,
    zone_id TEXT,
    record_name TEXT NOT NULL,
    record_id TEXT,
    allow_override INTEGER NOT NULL DEFAULT 1,
    ttl INTEGER NOT NULL DEFAULT 60,
    proxied INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    agent_id TEXT UNIQUE,
    group_id TEXT NOT NULL,
    name TEXT NOT NULL UNIQUE,
    public_ip TEXT NOT NULL,
    monthly_quota_bytes INTEGER NOT NULL,
    threshold_percent INTEGER NOT NULL,
    reset_day INTEGER NOT NULL DEFAULT 1,
    traffic_mode TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    auto_switch INTEGER NOT NULL DEFAULT 1,
    priority INTEGER NOT NULL DEFAULT 100,
    preferred_iface TEXT NOT NULL DEFAULT 'auto',
    report_interval_seconds INTEGER NOT NULL DEFAULT 60,
    online INTEGER NOT NULL DEFAULT 0,
    last_reported_at TIMESTAMP,
    first_seen_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agent_tokens (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    revoked_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(agent_id) REFERENCES nodes(agent_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_tokens_agent_hash
    ON agent_tokens(agent_id, token_hash);

CREATE TABLE IF NOT EXISTS join_codes (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL,
    code_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agent_reports (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    hostname TEXT NOT NULL,
    public_ip TEXT NOT NULL,
    iface TEXT NOT NULL,
    rx_bytes_total INTEGER NOT NULL,
    tx_bytes_total INTEGER NOT NULL,
    rx_delta INTEGER NOT NULL,
    tx_delta INTEGER NOT NULL,
    reported_at TIMESTAMP NOT NULL,
    agent_version TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_reports_agent_time
    ON agent_reports(agent_id, reported_at DESC);

CREATE TABLE IF NOT EXISTS traffic_counters (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL,
    cycle_start TEXT NOT NULL,
    rx_bytes INTEGER NOT NULL DEFAULT 0,
    tx_bytes INTEGER NOT NULL DEFAULT 0,
    used_bytes INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_traffic_counters_cycle
    ON traffic_counters(node_id, cycle_start);

CREATE TABLE IF NOT EXISTS dns_switch_history (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL,
    from_node_id TEXT,
    to_node_id TEXT,
    record_name TEXT NOT NULL,
    old_ip TEXT,
    new_ip TEXT,
    reason TEXT NOT NULL,
    status TEXT NOT NULL,
    error_message TEXT,
    switched_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    target_ref TEXT NOT NULL,
    message TEXT NOT NULL,
    status TEXT NOT NULL,
    error_message TEXT,
    sent_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
