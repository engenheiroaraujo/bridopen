create table if not exists public.users_personal_information (
    id bigserial primary key,
    user_id bigint not null references public.users(id) on delete cascade,
	first_name varchar(30) not null,
	last_name varchar(50) not null,
	phone_number varchar(20) null,
	address_id bigint null,
    created_at timestamp default current_timestamp,
    updated_at timestamp
);
