CREATE TABLE support_case (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    public_code TEXT NOT NULL,
    reporter_user_id UUID NOT NULL,
    chat_session_id UUID NOT NULL,
    support_issue_id UUID,
    technical_issue_id UUID,
    app_key TEXT NOT NULL CHECK (app_key = 'inaudit'),
    unit_id UUID,
    category TEXT,
    state TEXT NOT NULL DEFAULT 'novo' CHECK (state IN ('novo', 'coletando_contexto', 'aguardando_relator', 'em_analise', 'resposta_proposta', 'aguardando_confirmacao', 'concluido', 'em_investigacao_tecnica', 'aguardando_aprovacao', 'em_correcao', 'em_validacao', 'pronto_para_publicar', 'publicado', 'rejeitado', 'melhoria_registrada', 'bloqueado')),
    risk_level TEXT,
    confidence TEXT,
    resolution_type TEXT,
    resolution_summary TEXT,
    source_freshness_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    confirmed_at TIMESTAMPTZ,
    idempotency_key TEXT NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE support_case_transition (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    support_case_id UUID NOT NULL,
    previous_state TEXT,
    new_state TEXT NOT NULL,
    actor_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE support_case_sequence (
    workspace_id UUID NOT NULL,
    next_value BIGINT NOT NULL CHECK (next_value > 0)
);
