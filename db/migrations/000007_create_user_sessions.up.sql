create table public.user_sessions (
    id bigserial primary key,
    user_id bigint not null references public.users(id) on delete cascade,
    session_id uuid not null unique,
    user_agent text not null default '',
    ip_address inet,
    created_at timestamp default current_timestamp,
    last_seen_at timestamp default current_timestamp,
    revoked_at timestamp
);

create index idx_user_sessions_active
    on public.user_sessions (user_id, revoked_at, last_seen_at desc);