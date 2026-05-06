-- 008_default_cms_layout.sql
INSERT INTO cms_layouts (id, name, template_file, description)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Default',
    'blog-post.html',
    'Default layout for blog posts and pages'
) ON CONFLICT DO NOTHING;
