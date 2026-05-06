-- 004_funnels.sql
CREATE TYPE goal_type AS ENUM ('pageview', 'event', 'outbound');
CREATE TYPE funnel_match AS ENUM ('url', 'event', 'goal');

CREATE TABLE goals (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name    TEXT NOT NULL,
    type    goal_type NOT NULL,
    value   TEXT NOT NULL
);

CREATE TABLE funnels (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id    UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE funnel_steps (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    funnel_id  UUID NOT NULL REFERENCES funnels(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    match_type funnel_match NOT NULL,
    value      TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    UNIQUE (funnel_id, position)
);
