drop procedure if exists auditoria.sp_events_insert(
    varchar,
    bigint,
    varchar,
    varchar,
    varchar,
    varchar,
    varchar,
    varchar
);
drop table if exists auditoria.events;
drop table if exists auditoria.event_types;
drop schema if exists auditoria;
