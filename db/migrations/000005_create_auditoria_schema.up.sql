create schema if not exists auditoria;

create table if not exists auditoria.event_types (
    id_event_type bigserial primary key,
    event_code varchar(20) not null,
    category varchar(30) not null,
    name varchar(100) not null,
    default_message varchar(500) not null,
    level varchar(20) not null,
    active boolean not null default true,

    constraint uq_event_types_event_code unique (event_code)
);

create table if not exists auditoria.events (
    id_events bigserial primary key,
    id_event_type bigint null,
    user_id bigint null,
    ip varchar(45) null,
    date_time timestamp default current_timestamp,
    operation varchar(20) null,
    object varchar(150) null,
    module varchar(50) null,
    message varchar(500) null,
    result varchar(20) null,

    constraint fk_events_event_types
        foreign key (id_event_type)
        references auditoria.event_types (id_event_type)
);

create or replace procedure auditoria.sp_events_insert(
    p_event_code varchar(20),
    p_user_id bigint default null,
    p_ip varchar(45) default null,
    p_operation varchar(20) default null,
    p_object varchar(150) default null,
    p_module varchar(50) default null,
    p_message varchar(500) default null,
    p_result varchar(20) default null
)
language plpgsql
as $$
declare
    v_rows integer;
begin
    insert into auditoria.events (
		id_event_type
	,	user_id
	,	ip
	,	operation
	,	object
	,	module
	,	message
	,	result
    )
    select
		event_types.id_event_type
	,	p_user_id
	,	p_ip
	,	p_operation
	,	p_object
	,	p_module
	,	coalesce(p_message, event_types.default_message)
	,	p_result
    from 
		auditoria.event_types
    where 1=1
		and event_types.event_code = p_event_code
		and event_types.active = true;

    get diagnostics v_rows = row_count;
    if v_rows = 0 then
        raise exception 'event_code de auditoria nao encontrado ou inativo: %', p_event_code;
    end if;
end;
$$;

insert into auditoria.event_types (event_code, category, name, default_message, level)
values
    -- Autenticação
    ('AUTH001', 'autenticacao', 'login realizado', 'usuário autenticado com sucesso', 'info'),
    ('AUTH002', 'autenticacao', 'falha no login', 'falha na autenticação: usuário ou senha inválidos', 'warning'),
    ('AUTH003', 'autenticacao', 'logout realizado', 'sessão encerrada pelo usuário', 'info'),
    ('AUTH004', 'autenticacao', 'sessão expirada', 'sessão encerrada por expiração de tempo', 'warning'),
    ('AUTH005', 'autenticacao', 'conta bloqueada', 'conta bloqueada por excesso de tentativas', 'warning'),
    ('AUTH006', 'autenticacao', 'conta desbloqueada', 'conta desbloqueada', 'info'),


    -- Autenticação Multifator
    ('MFA001', 'autenticacao', 'mfa validado', 'mfa validado com sucesso', 'info'),
    ('MFA002', 'autenticacao', 'mfa inválido', 'falha na autenticação: código mfa inválido', 'warning'),
    ('MFA003', 'autenticacao', 'configuracao mfa iniciada', 'configuracao de mfa iniciada', 'info'),
    ('MFA004', 'autenticacao', 'mfa ativado', 'verificacao em duas etapas ativada', 'info'),
    ('MFA005', 'autenticacao', 'falha ao ativar mfa', 'falha ao ativar verificacao em duas etapas', 'warning'),
    ('MFA006', 'autenticacao', 'login mfa validado', 'login com mfa validado com sucesso', 'info'),
    ('MFA007', 'autenticacao', 'falha no login mfa', 'falha na validacao de mfa durante login', 'warning'),
    ('MFA008', 'autenticacao', 'mfa desativado', 'verificacao em duas etapas desativada', 'warning'),
    ('MFA009', 'autenticacao', 'falha ao desativar mfa', 'falha ao desativar verificacao em duas etapas', 'warning'),
    ('MFA010', 'autenticacao', 'desativacao mfa iniciada', 'desativacao de verificacao em duas etapas iniciada', 'info'),

    -- Usuários e permissões
    ('USER001', 'usuario', 'usuário cadastrado', 'usuário cadastrado', 'info'),
    ('USER002', 'usuario', 'usuário atualizado', 'usuário atualizado', 'info'),
    ('USER003', 'usuario', 'usuário removido', 'usuário removido', 'warning'),
    ('USER004', 'usuario', 'senha alterada', 'senha alterada com sucesso', 'info'),
    ('USER005', 'usuario', 'redefinição de senha solicitada', 'solicitação de redefinição de senha realizada', 'info'),
    ('PERM001', 'permissao', 'perfil atribuído', 'perfil de acesso atribuído', 'info'),
    ('PERM002', 'permissao', 'perfil removido', 'perfil de acesso removido', 'warning'),
    ('PERM003', 'permissao', 'permissão concedida', 'permissão concedida', 'info'),
    ('PERM004', 'permissao', 'permissão revogada', 'permissão revogada', 'warning'),
    ('PERM005', 'permissao', 'acesso não autorizado', 'tentativa de acesso não autorizada', 'warning'),

    -- Dados
    ('DATA001', 'dados', 'registro incluído', 'registro incluído', 'info'),
    ('DATA002', 'dados', 'registro alterado', 'registro alterado', 'info'),
    ('DATA003', 'dados', 'registro excluído', 'registro excluído', 'warning'),
    ('DATA004', 'dados', 'registro consultado', 'registro consultado', 'info'),
    ('DATA005', 'dados', 'registro restaurado', 'registro restaurado', 'info'),
    ('DATA006', 'dados', 'importação concluída', 'importação concluída com sucesso', 'info'),
    ('DATA007', 'dados', 'falha na importação', 'falha na importação de dados', 'error'),
    ('DATA008', 'dados', 'exportação concluída', 'exportação concluída com sucesso', 'info'),
    ('DATA009', 'dados', 'falha na exportação', 'falha na exportação de dados', 'error'),

    -- Arquivos
    ('FILE001', 'arquivo', 'arquivo enviado', 'arquivo enviado', 'info'),
    ('FILE002', 'arquivo', 'arquivo atualizado', 'arquivo atualizado', 'info'),
    ('FILE003', 'arquivo', 'arquivo removido', 'arquivo removido', 'warning'),
    ('FILE004', 'arquivo', 'download realizado', 'download de arquivo realizado', 'info'),
    ('FILE005', 'arquivo', 'arquivo inválido', 'arquivo rejeitado por formato inválido', 'warning'),
    ('FILE006', 'arquivo', 'arquivo excedeu tamanho', 'arquivo rejeitado por exceder o tamanho permitido', 'warning'),

    -- Documentos
    ('DOC001', 'documento', 'documento assinado', 'assinatura de documento realizada', 'info'),
    ('DOC002', 'documento', 'documento validado', 'validação de documento realizada', 'info'),
    ('DOC003', 'documento', 'falha na validação', 'falha na validação de documento', 'error'),
    ('DOC004', 'documento', 'documento cancelado', 'documento cancelado', 'warning'),

    -- Integrações
    ('INT001', 'integracao', 'integração iniciada', 'integração iniciada', 'info'),
    ('INT002', 'integracao', 'integração concluída', 'integração concluída com sucesso', 'info'),
    ('INT003', 'integracao', 'falha na integração', 'falha na integração', 'error'),
    ('INT004', 'integracao', 'timeout externo', 'timeout na comunicação com serviço externo', 'error'),
    ('INT005', 'integracao', 'retorno inválido', 'retorno inválido recebido do serviço externo', 'error'),
    ('INT006', 'integracao', 'serviço externo indisponível', 'serviço externo indisponível', 'error'),

    -- Sistema
    ('SYS001', 'sistema', 'sistema iniciado', 'sistema iniciado', 'info'),
    ('SYS002', 'sistema', 'sistema finalizado', 'sistema finalizado', 'info'),
    ('SYS003', 'sistema', 'serviço iniciado', 'serviço iniciado', 'info'),
    ('SYS004', 'sistema', 'serviço interrompido', 'serviço interrompido', 'warning'),
    ('SYS005', 'sistema', 'configuração alterada', 'configuração alterada', 'warning'),
    ('SYS006', 'sistema', 'versão atualizada', 'atualização de versão realizada', 'info'),

    -- Backup
    ('BKP001', 'backup', 'backup iniciado', 'backup iniciado', 'info'),
    ('BKP002', 'backup', 'backup concluído', 'backup concluído com sucesso', 'info'),
    ('BKP003', 'backup', 'falha no backup', 'falha na execução do backup', 'error'),
    ('BKP004', 'backup', 'restauração iniciada', 'restauração iniciada', 'warning'),
    ('BKP005', 'backup', 'restauração concluída', 'restauração realizada com sucesso', 'info'),
    ('BKP006', 'backup', 'falha na restauração', 'falha na restauração', 'error'),

    -- Segurança, Criptografia e Ameaças
    ('SEC001', 'seguranca', 'acesso bloqueado', 'tentativa de acesso bloqueada', 'warning'),
    ('SEC002', 'seguranca', 'recurso restrito', 'tentativa de acesso a recurso restrito', 'warning'),
    ('SEC003', 'seguranca', 'token gerado', 'token gerado', 'info'),
    ('SEC004', 'seguranca', 'token revogado', 'token revogado', 'warning'),
    ('SEC005', 'seguranca', 'certificado validado', 'certificado validado', 'info'),
    ('SEC006', 'seguranca', 'certificado expirado', 'certificado expirado', 'error'),
    ('SEC007', 'seguranca', 'chave criptográfica gerada', 'nova chave criptográfica gerada', 'info'),
    ('SEC008', 'seguranca', 'chave criptográfica revogada', 'chave criptográfica revogada ou rotacionada', 'warning'),
    ('SEC009', 'seguranca', 'incidente de segurança reportado', 'incidente crítico de segurança identificado', 'error'),
    ('FILE007', 'arquivo', 'ameaça detectada', 'arquivo rejeitado por suspeita de malware', 'error'),

    -- Erros
    ('ERR001', 'erro', 'erro de validação', 'erro de validação de dados', 'warning'),
    ('ERR002', 'erro', 'erro de processamento', 'erro de processamento', 'error'),
    ('ERR003', 'erro', 'erro no banco de dados', 'erro de conexão com banco de dados', 'error'),
    ('ERR004', 'erro', 'erro externo', 'erro de conexão com serviço externo', 'error'),
    ('ERR005', 'erro', 'erro inesperado', 'erro inesperado', 'error'),
    ('ERR006', 'erro', 'operação cancelada', 'operação cancelada', 'warning'),
    ('ERR007', 'erro', 'recurso não encontrado', 'recurso não encontrado', 'warning'),
    ('ERR008', 'erro', 'limite excedido', 'limite operacional excedido', 'warning'),

    -- Privacidade e LGPD
    ('LGPD001', 'privacidade', 'consentimento concedido', 'usuário concedeu consentimento para tratamento de dados', 'info'),
    ('LGPD002', 'privacidade', 'consentimento revogado', 'usuário revogou consentimento para tratamento de dados', 'warning'),
    ('LGPD003', 'privacidade', 'dados anonimizados', 'registro de dados pessoais foi anonimizado', 'info'),
    ('LGPD004', 'privacidade', 'solicitação de titular', 'registro de solicitação de direitos do titular (ex: exclusão, portabilidade)', 'info'),
    ('LGPD005', 'privacidade', 'termo aceito', 'política de privacidade ou termo de uso aceito', 'info'),

    -- Auditoria
    ('AUD001', 'auditoria', 'trilha consultada', 'consulta realizada às trilhas de auditoria', 'warning'),
    ('AUD002', 'auditoria', 'trilha exportada', 'exportação de trilhas de auditoria realizada', 'warning')
on conflict (event_code) do nothing;

CREATE INDEX IF NOT EXISTS ix_events_date_time
ON auditoria.events (date_time);

CREATE INDEX IF NOT EXISTS ix_events_id_event_type
ON auditoria.events (id_event_type);

CREATE INDEX IF NOT EXISTS ix_events_user_id
ON auditoria.events (user_id);
