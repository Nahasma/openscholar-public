-- +goose Up

-- 进化案例：记录用户反馈
CREATE TABLE IF NOT EXISTS evolution_cases (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    skill_id        TEXT,
    user_request    TEXT NOT NULL,
    agent_output    TEXT,
    feedback        TEXT NOT NULL,
    feedback_type   TEXT NOT NULL DEFAULT 'explicit',
    status          TEXT NOT NULL DEFAULT 'pending',
    resolution      TEXT,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_evo_cases_status ON evolution_cases(status);
CREATE INDEX IF NOT EXISTS idx_evo_cases_skill ON evolution_cases(skill_id);

-- 技能快照：进化前的完整备份，支持回滚
CREATE TABLE IF NOT EXISTS skill_snapshots (
    id          TEXT PRIMARY KEY,
    skill_id    TEXT NOT NULL,
    version     INTEGER NOT NULL,
    content     TEXT NOT NULL,
    reason      TEXT,
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshots_skill ON skill_snapshots(skill_id);

-- +goose Down
DROP TABLE IF EXISTS skill_snapshots;
DROP TABLE IF EXISTS evolution_cases;
