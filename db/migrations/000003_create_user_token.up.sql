create table if not exists public.users_confirmation_tokens (
    id bigserial primary key,
    user_id bigint not null references public.users(id) on delete cascade,
    token text not null,
    token_hash text not null,
    purpose text not null default 'email_confirmation',
    confirmed boolean not null default false,
    expires_at timestamp not null default (current_timestamp + interval '24 hours'),
    created_at timestamp default current_timestamp,
    updated_at timestamp
);

create unique index if not exists idx_users_confirmation_tokens_token_hash
    on public.users_confirmation_tokens (token_hash);

create index if not exists idx_users_confirmation_tokens_expires_at
    on public.users_confirmation_tokens (expires_at);
