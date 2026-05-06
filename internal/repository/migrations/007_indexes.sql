-- 007_indexes.sql
CREATE INDEX ON sites (owner_id);
CREATE INDEX ON sites (domain);
CREATE INDEX ON site_members (user_id);
CREATE INDEX ON site_members (site_id);
CREATE INDEX ON invitations (token);
CREATE INDEX ON invitations (email);
CREATE INDEX ON revoked_tokens (expires_at);
CREATE INDEX ON password_reset_tokens (user_id);
CREATE INDEX ON cms_pages (slug);
CREATE INDEX ON cms_pages (status, published_at DESC);
CREATE INDEX ON audit_log (actor_id, created_at DESC);
CREATE INDEX ON goals (site_id);
CREATE INDEX ON funnels (site_id);
