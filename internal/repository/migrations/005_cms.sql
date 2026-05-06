-- 005_cms.sql
CREATE TYPE page_type AS ENUM ('blog', 'page');
CREATE TYPE page_status AS ENUM ('draft', 'published');

CREATE TABLE cms_layouts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    template_file TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE cms_pages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    layout_id        UUID NOT NULL REFERENCES cms_layouts(id),
    author_id        UUID NOT NULL REFERENCES users(id),
    title            TEXT NOT NULL,
    slug             TEXT NOT NULL UNIQUE,
    type             page_type NOT NULL DEFAULT 'blog',
    content_html     TEXT NOT NULL DEFAULT '',
    excerpt          TEXT NOT NULL DEFAULT '',
    cover_image_url  TEXT NOT NULL DEFAULT '',
    meta_title       TEXT NOT NULL DEFAULT '',
    meta_description TEXT NOT NULL DEFAULT '',
    status           page_status NOT NULL DEFAULT 'draft',
    published_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE cms_tags (
    id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE
);

CREATE TABLE cms_page_tags (
    page_id UUID NOT NULL REFERENCES cms_pages(id) ON DELETE CASCADE,
    tag_id  UUID NOT NULL REFERENCES cms_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (page_id, tag_id)
);
