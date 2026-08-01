"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";

const stateLabels: Record<string, string> = {
  novo: "Novo",
  aguardando_relator: "Aguardando a líder",
  em_analise: "Em análise",
  aguardando_confirmacao: "Aguardando confirmação",
  concluido: "Concluído",
  em_investigacao_tecnica: "Investigação técnica",
  aguardando_aprovacao: "Aguardando sua aprovação",
  em_correcao: "Em correção",
  em_validacao: "Em validação",
  pronto_para_publicar: "Pronto para publicar",
  publicado: "Publicado",
  rejeitado: "Rejeitado",
  bloqueado: "Bloqueado",
};

export default function SupportAdminPage() {
  const workspaceId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const casesKey = ["support-admin", workspaceId, "cases"];
  const metricsKey = ["support-admin", workspaceId, "metrics"];
  const cases = useQuery({
    queryKey: casesKey,
    queryFn: () => api.listSupportAdminCases(),
    enabled: workspaceId.length > 0,
    refetchInterval: 30_000,
  });
  const metrics = useQuery({
    queryKey: metricsKey,
    queryFn: () => api.getSupportAdminMetrics(),
    enabled: workspaceId.length > 0,
    refetchInterval: 30_000,
  });
  const detail = useQuery({
    queryKey: ["support-admin", workspaceId, "case", selectedId],
    queryFn: () => api.getSupportAdminCase(selectedId ?? ""),
    enabled: Boolean(selectedId),
  });
  const decision = useMutation({
    mutationFn: ({ caseId, approved }: { caseId: string; approved: boolean }) =>
      api.decideSupportExecution(caseId, approved),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: casesKey }),
        queryClient.invalidateQueries({ queryKey: metricsKey }),
        queryClient.invalidateQueries({
          queryKey: ["support-admin", workspaceId, "case", selectedId],
        }),
      ]);
    },
  });

  const cards = [
    ["Casos", metrics.data?.total_cases ?? 0],
    ["Respostas automáticas", metrics.data?.automatic_answers ?? 0],
    ["Escalados", metrics.data?.technical_escalations ?? 0],
    ["Confirmados", metrics.data?.confirmed_resolutions ?? 0],
    ["Reabertos", metrics.data?.reopened_cases ?? 0],
    ["Aguardando aprovação", metrics.data?.awaiting_approval ?? 0],
  ];

  return (
    <main className="mx-auto min-h-svh w-full max-w-7xl space-y-6 p-4 md:p-8">
      <header>
        <h1 className="text-title-lg font-semibold">Central de Suporte Innova</h1>
        <p className="mt-1 text-body text-muted-foreground">
          Fila interna, evidências, decisões e resultado do Concierge.
        </p>
      </header>

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-6">
        {cards.map(([label, value]) => (
          <article key={label} className="rounded-lg border bg-card p-4">
            <p className="text-caption text-muted-foreground">{label}</p>
            <p className="mt-2 text-title-lg font-semibold">{value}</p>
          </article>
        ))}
      </section>

      <section className="grid gap-6 lg:grid-cols-[22rem_minmax(0,1fr)]">
        <div className="rounded-xl border bg-card p-4">
          <h2 className="font-semibold">Atendimentos</h2>
          {cases.isPending ? (
            <p className="mt-4 text-body text-muted-foreground">Carregando…</p>
          ) : cases.isError ? (
            <p className="mt-4 text-body text-destructive">Não foi possível carregar a fila.</p>
          ) : (
            <ul className="mt-4 space-y-2">
              {(cases.data ?? []).map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedId(item.id)}
                    aria-current={selectedId === item.id ? "page" : undefined}
                    className="w-full rounded-md border p-3 text-left hover:bg-muted aria-[current=page]:border-primary"
                  >
                    <span className="font-medium">{item.public_code}</span>
                    <span className="mt-1 block text-caption text-muted-foreground">
                      {stateLabels[item.state] ?? item.state}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="min-h-[30rem] rounded-xl border bg-card p-5">
          {!selectedId ? (
            <p className="text-body text-muted-foreground">Selecione um atendimento.</p>
          ) : detail.isPending ? (
            <p className="text-body text-muted-foreground">Carregando atendimento…</p>
          ) : detail.isError || !detail.data ? (
            <p className="text-body text-destructive">Não foi possível carregar o atendimento.</p>
          ) : (
            <div className="space-y-5">
              <div>
                <h2 className="text-title font-semibold">{detail.data.case.public_code}</h2>
                <p className="text-body text-muted-foreground">
                  {stateLabels[detail.data.case.state] ?? detail.data.case.state}
                </p>
              </div>
              {detail.data.case.resolution_summary && (
                <section className="rounded-lg border p-4">
                  <h3 className="font-medium">Diagnóstico</h3>
                  <p className="mt-2 whitespace-pre-wrap text-body">{detail.data.case.resolution_summary}</p>
                </section>
              )}
              {detail.data.case.approval_summary && (
                <section className="rounded-lg border border-primary/40 p-4">
                  <h3 className="font-medium">Proposta r{detail.data.case.approval_revision}</h3>
                  <p className="mt-2 whitespace-pre-wrap text-body">{detail.data.case.approval_summary}</p>
                </section>
              )}
              {detail.data.case.state === "aguardando_aprovacao" && (
                <div className="flex gap-3">
                  <button
                    type="button"
                    disabled={decision.isPending}
                    onClick={() => decision.mutate({ caseId: detail.data.case.id, approved: true })}
                    className="rounded-md bg-primary px-4 py-2 text-primary-foreground disabled:opacity-50"
                  >
                    Aprovar execução
                  </button>
                  <button
                    type="button"
                    disabled={decision.isPending}
                    onClick={() => decision.mutate({ caseId: detail.data.case.id, approved: false })}
                    className="rounded-md border px-4 py-2 disabled:opacity-50"
                  >
                    Rejeitar
                  </button>
                </div>
              )}
              <section>
                <h3 className="font-medium">Conversa</h3>
                <div className="mt-3 space-y-3">
                  {detail.data.messages.map((message) => (
                    <article key={message.id} className="rounded-lg border p-3 text-body">
                      <p className="text-caption font-medium text-muted-foreground">
                        {message.role === "user" ? "Líder" : "Concierge"}
                      </p>
                      <p className="mt-1 whitespace-pre-wrap">{message.content}</p>
                      {message.attachments && message.attachments.length > 0 && (
                        <ul className="mt-3 space-y-2">
                          {message.attachments.map((attachment) => (
                            <li key={attachment.id}>
                              <a
                                className="underline"
                                href={attachment.download_url}
                                target="_blank"
                                rel="noreferrer"
                              >
                                {attachment.filename}
                              </a>
                            </li>
                          ))}
                        </ul>
                      )}
                    </article>
                  ))}
                </div>
              </section>
            </div>
          )}
        </div>
      </section>
    </main>
  );
}
