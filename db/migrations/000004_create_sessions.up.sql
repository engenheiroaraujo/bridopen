create table if not exists public.sessions (
    token text primary key,
    data bytea not null,
    expiry timestamptz not null
);

create index if not exists idx_sessions_expiry on public.sessions (expiry);
