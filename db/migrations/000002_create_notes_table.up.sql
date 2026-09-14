create table if not exists public.notes (
    id bigserial primary key,
    user_id bigint not null,
    pinned boolean not null default false,
    archived_at timestamp null,
    deleted_at timestamp null,
    created_at timestamp default current_timestamp,
    updated_at timestamp default current_timestamp,

    constraint fk_notes_user
        foreign key (user_id)
        references public.users(id)
        on delete cascade,

    constraint uq_notes_id_user
        unique (id, user_id)
);

create table if not exists public.notes_content (
    id bigserial primary key,
    note_id bigint not null,
    title text not null,
    content text,
    color text not null,

    constraint chk_notes_content_color_not_blank check (btrim(color) <> ''),

    constraint fk_notes_content_note
        foreign key (note_id)
        references public.notes(id)
        on delete cascade,

    constraint uq_notes_content_note
        unique (note_id)
);

create extension if not exists unaccent with schema public;
create extension if not exists pg_trgm with schema public;

create or replace function public.qn_unaccent(value text)
returns text
language sql
immutable
parallel safe
strict
as $$
    select public.unaccent('public.unaccent'::regdictionary, value);
$$;

create table if not exists public.note_tags (
    id bigserial primary key,
    user_id bigint not null,
    name text not null,
    slug text not null,
    color text,
    created_at timestamp default current_timestamp,
    updated_at timestamp,

    constraint uq_note_tags_user_slug unique (user_id, slug),
    constraint chk_note_tags_name_not_blank check (btrim(name) <> ''),
    constraint chk_note_tags_slug_not_blank check (btrim(slug) <> ''),
    constraint fk_note_tags_user
        foreign key (user_id)
        references public.users(id)
        on delete cascade
);

create table if not exists public.note_tag_links (
    note_id bigint not null references public.notes(id) on delete cascade,
    tag_id bigint not null references public.note_tags(id) on delete cascade,
    created_at timestamp default current_timestamp,

    constraint pk_note_tag_links primary key (note_id, tag_id)
);

create table if not exists public.note_attachments (
    id bigserial primary key,
    note_id bigint not null,
    user_id bigint not null,
    original_name text not null,
    storage_key text not null,
    mime_type text not null,
    size_bytes bigint not null,
    checksum_sha256 text,
    created_at timestamp default current_timestamp,

    constraint fk_note_attachments_note_user
        foreign key (note_id, user_id)
        references public.notes(id, user_id)
        on delete cascade,

    constraint uq_note_attachments_storage_key
        unique (storage_key),

    constraint chk_note_attachments_original_name_not_blank
        check (btrim(original_name) <> ''),

    constraint chk_note_attachments_storage_key_not_blank
        check (btrim(storage_key) <> ''),

    constraint chk_note_attachments_mime_type_not_blank
        check (btrim(mime_type) <> ''),

    constraint chk_note_attachments_size_positive
        check (size_bytes > 0),

    constraint chk_note_attachments_checksum_sha256
        check (checksum_sha256 is null or checksum_sha256 ~ '^[a-f0-9]{64}$')
);

create index if not exists idx_notes_user_state
    on public.notes (user_id, deleted_at, archived_at, pinned desc, updated_at desc, id desc);

create index if not exists idx_notes_user_recent
    on public.notes (user_id, deleted_at, archived_at, updated_at desc, id desc);

create index if not exists idx_notes_content_color_note
    on public.notes_content (color, note_id);

create index if not exists idx_note_tags_user_name
    on public.note_tags (user_id, name);

create index if not exists idx_note_tag_links_tag_id
    on public.note_tag_links (tag_id, note_id);

create index if not exists idx_note_attachments_note_created
    on public.note_attachments (note_id, created_at desc, id desc);

create index if not exists idx_note_attachments_user_created
    on public.note_attachments (user_id, created_at desc, id desc);

create index if not exists idx_notes_content_title_trgm
    on public.notes_content using gin (public.qn_unaccent(title) gin_trgm_ops);

create index if not exists idx_notes_content_body_trgm
    on public.notes_content using gin (public.qn_unaccent(content) gin_trgm_ops);

create index if not exists idx_note_tags_name_trgm
    on public.note_tags using gin (public.qn_unaccent(name) gin_trgm_ops);
