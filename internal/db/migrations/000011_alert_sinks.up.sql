-- Per-project alert destinations and their delivery log.
CREATE TABLE IF NOT EXISTS alert_sinks (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    kind        VARCHAR(20) NOT NULL CHECK (kind IN ('webhook','slack','email')),
    config      JSONB NOT NULL DEFAULT '{}',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_alert_sinks_project ON alert_sinks(project_id);

CREATE TABLE IF NOT EXISTS alert_deliveries (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sink_id      UUID NOT NULL REFERENCES alert_sinks(id) ON DELETE CASCADE,
    alert_id     UUID NOT NULL,
    alert_type   VARCHAR(20) NOT NULL,
    attempt      INT NOT NULL,
    status       VARCHAR(10) NOT NULL CHECK (status IN ('ok','error','dead')),
    http_status  INT NOT NULL DEFAULT 0,
    error        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_alert_deliveries_sink ON alert_deliveries(sink_id, created_at DESC);
