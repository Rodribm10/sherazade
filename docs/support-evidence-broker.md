# Support evidence broker

The support Concierge uses a hybrid source model:

1. versioned product knowledge for stable operating guidance;
2. read-only code evidence from the revision deployed for the selected app;
3. read-only live data evidence scoped to the reporter, workspace, and app.

Multica does not receive repository write tokens, database credentials, a
shell, or arbitrary SQL access. It sends the case context to one
operator-configured broker and receives bounded, attributed evidence. The
Concierge may explain what the evidence proves, but correction remains a
separate approval-gated workflow.

## HTTP contract

Configure the full broker endpoint with `MULTICA_SUPPORT_EVIDENCE_URL` and an
optional server-side bearer token with `MULTICA_SUPPORT_EVIDENCE_TOKEN`.
Redirects are not followed so the token cannot be forwarded to another host.
Remote endpoints must use HTTPS; loopback HTTP is accepted for development.

Request:

```json
{
  "case_id": "uuid",
  "workspace_id": "uuid",
  "reporter_user_id": "uuid",
  "reporter_email": "lider@empresa.com.br",
  "reporter_name": "Nome da lider",
  "application_key": "inaudit",
  "unit_id": "uuid opcional",
  "conversation": [
    {"role": "user", "content": "Por que esta tarefa nao pontuou?"}
  ]
}
```

Response:

```json
{
  "sources": [
    {
      "kind": "code",
      "title": "Regra implantada",
      "reference": "github://inaudit/src/score.ts:42",
      "content": "A regra considera tarefas concluidas dentro do prazo.",
      "observed_at": "2026-08-01T16:00:00Z",
      "deployed_revision": "abc123"
    },
    {
      "kind": "data",
      "title": "Explicacao da pontuacao atual",
      "reference": "rpc://explain_black_belt_score_v1",
      "content": "A tarefa foi concluida depois do prazo configurado.",
      "observed_at": "2026-08-01T16:00:01Z"
    }
  ],
  "limitations": []
}
```

`kind` is restricted to `knowledge`, `code`, or `data`. Every source must have
a reference and content. Multica rejects unknown fields, redirects, oversized
responses, unsupported source types, and unattributed evidence. Returned text
is redacted and bounded before it reaches the model. `reference` is retained
for internal audit only; the leader-facing answer uses the source title and
observation time without exposing repository paths or database identifiers.

## Connector requirements

Each application adapter must enforce these boundaries before returning data:

- resolve the reporter's allowed unit and role server-side;
- expose allowlisted views or parameterized diagnostic functions, never
  arbitrary SQL;
- use a database identity with `SELECT`/`EXECUTE` only on those objects;
- enforce statement timeout, row limit, and sensitive-column denylist;
- search code at the deployed revision, not the repository default branch;
- return source references, observation time, and deployed revision;
- keep investigation and mutation under separate identities and APIs.

Questions about scores, payroll, finance, or permissions may be explained only
when deterministic rule and live-data evidence agree. Missing or conflicting
evidence produces a context request or technical escalation; it never grants
the Concierge permission to change the application.

## First InAudit adapter

The candidate adapter lives in the InAudit repository as the Supabase Edge
Function `support-evidence-broker`. It currently supports one deliberately
narrow tool: explaining the reporter's own Black Belt score for
`conferencia_quartos`.

The adapter resolves the immutable Multica login email to one active InAudit
profile, lists only that profile's allowed units, requires an unambiguous unit,
and returns a sanitized deterministic explanation. Operational notes, evidence
URLs, audit IDs, snapshot IDs, and arbitrary subject IDs never cross the
boundary.

After the InAudit security gate and controlled deployment are complete, point
Multica to:

```dotenv
MULTICA_SUPPORT_EVIDENCE_URL=https://acdvblhzzaneddlxqyst.supabase.co/functions/v1/support-evidence-broker
MULTICA_SUPPORT_EVIDENCE_TOKEN=<same dedicated token stored in the InAudit function>
```

Do not enable this configuration yet. InAudit PR
`Rodribm10/InAudit-Antigravity#202`, commit `43413f1`, now contains the candidate
hardening migration: direct anonymous writes and anonymous execution of the
Conferência de Quartos RPCs are removed, while authenticated manual actions go
through permissioned RPCs. The migration is not live yet. Activation still
requires a controlled apply, a real-JWT regression, a fresh privilege check and
an end-to-end broker smoke test. Until then the Concierge continues to use
versioned knowledge and reports live evidence as unavailable.

Residual legacy debt: anonymous `SELECT` remains temporarily because current
InAudit presentations and reports still read Black Belt through the publishable
client. Removing that read path is a separate RLS migration after those
consumers move to authenticated clients.
