create table if not exists public.users (
    id bigserial primary key,
    email text not null unique,
    recovery_email text null unique,
    pending_recovery_email text null unique,
    password text not null,
    active boolean not null default false,
    two_factor_enabled boolean not null default false,
    two_factor_method text,
    two_factor_totp_secret_encrypted bytea,
    two_factor_enabled_at timestamp,
    two_factor_disabled_at timestamp,
    created_at timestamp default current_timestamp,
    updated_at timestamp,

    constraint chk_users_two_factor_method
        check (two_factor_method is null or two_factor_method in ('email', 'totp')),
    constraint chk_users_two_factor_enabled_method
        check (
            (two_factor_enabled = false and two_factor_method is null)
            or (two_factor_enabled = true and two_factor_method is not null)
        ),
    constraint chk_users_two_factor_enabled_at
        check (two_factor_enabled = false or two_factor_enabled_at is not null),
    constraint chk_users_two_factor_totp_secret
        check (two_factor_method is distinct from 'totp' or two_factor_totp_secret_encrypted is not null),
    constraint chk_users_two_factor_email_secret
        check (two_factor_method is distinct from 'email' or two_factor_totp_secret_encrypted is null),
    constraint chk_users_two_factor_disabled_secret
        check (two_factor_enabled = true or two_factor_totp_secret_encrypted is null)
);

create table if not exists public.users_two_factor_challenges (
    id bigserial primary key,
    user_id bigint not null references public.users(id) on delete cascade,
    method text not null,
    purpose text not null,
    code_hash text,
    totp_secret_encrypted bytea,
    expires_at timestamp not null,
    consumed_at timestamp,
    attempt_count integer not null default 0,
    created_at timestamp default current_timestamp,
    updated_at timestamp,

    constraint chk_users_two_factor_challenges_method
        check (method in ('email', 'totp')),
    constraint chk_users_two_factor_challenges_purpose
        check (purpose in ('enable', 'login', 'disable')),
    constraint chk_users_two_factor_challenges_code_or_secret
        check (
            (method = 'email' and code_hash is not null and totp_secret_encrypted is null)
            or (method = 'totp' and purpose = 'enable' and code_hash is null and totp_secret_encrypted is not null)
            or (method = 'totp' and purpose in ('login', 'disable') and code_hash is null and totp_secret_encrypted is null)
        )
);

create table if not exists public.users_recovery_codes (
    id bigserial primary key,
    user_id bigint not null references public.users(id) on delete cascade,
    code_hash text not null,
    used_at timestamp null,
    created_at timestamp not null default current_timestamp
);

create index if not exists idx_users_recovery_codes_lookup
    on public.users_recovery_codes (user_id, code_hash)
    where used_at is null;

create index if not exists idx_users_two_factor_challenges_lookup
    on public.users_two_factor_challenges (user_id, method, purpose, expires_at)
    where consumed_at is null;

create index if not exists idx_users_two_factor_challenges_expires_at
    on public.users_two_factor_challenges (expires_at);
