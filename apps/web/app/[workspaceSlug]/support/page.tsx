"use client";

import type { ChangeEvent, FormEvent } from "react";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  supportMessagesOptions,
  supportSessionsOptions,
  useCreateSupportSession,
  useSendSupportMessage,
  useSupportResolutionFeedback,
} from "@multica/core/support";

const supportStateLabel: Record<string, string> = {
  novo: "Recebido — aguardando análise",
  coletando_contexto: "Coletando informações",
  aguardando_relator: "Aguardando sua resposta",
  em_analise: "Em análise",
  resposta_proposta: "Resposta do Concierge disponível",
  aguardando_confirmacao: "Aguardando sua confirmação",
  concluido: "Concluído",
  em_investigacao_tecnica: "Encaminhado para investigação técnica",
  aguardando_aprovacao: "Aguardando aprovação do Rodrigo",
  bloqueado: "Bloqueado",
};

export default function SupportPage() {
  const workspaceId = useWorkspaceId();
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(
    null,
  );
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);
	const [files, setFiles] = useState<File[]>([]);
  const [idempotencyKey, setIdempotencyKey] = useState(() =>
    crypto.randomUUID(),
  );

  const sessionsQuery = useQuery(supportSessionsOptions(workspaceId));
  const sessions = sessionsQuery.data ?? [];
  const selectedSession =
    sessions.find((session) => session.session_id === selectedSessionId) ??
    null;
  const messagesQuery = useQuery(
    supportMessagesOptions(workspaceId, selectedSession?.session_id ?? ""),
  );
  const messages = messagesQuery.data ?? [];
  const createSession = useCreateSupportSession(workspaceId);
  const sendMessage = useSendSupportMessage(workspaceId);
  const resolutionFeedback = useSupportResolutionFeedback(workspaceId);
  const submitting =
    createSession.isPending ||
    sendMessage.isPending ||
    resolutionFeedback.isPending;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const content = description.trim();
    if (!content || submitting) return;

    setError(null);
    try {
		let targetSessionId = selectedSession?.session_id ?? "";
      if (selectedSession) {
		const attachments = await Promise.all(
			files.map((file) => api.uploadSupportAttachment(targetSessionId, file)),
		);
        const sent = await sendMessage.mutateAsync({
			sessionId: targetSessionId,
          content,
			attachmentIds: attachments.map((attachment) => attachment.id),
        });
        if (!sent.id) throw new Error("invalid support message response");
      } else {
        const created = await createSession.mutateAsync({
          idempotency_key: idempotencyKey,
          description: content,
			defer_analysis: files.length > 0,
        });
        if (!created.id || !created.session_id) {
          throw new Error("invalid support session response");
        }
        setSelectedSessionId(created.session_id);
		targetSessionId = created.session_id;
		if (files.length > 0) {
			const attachments = await Promise.all(
				files.map((file) => api.uploadSupportAttachment(targetSessionId, file)),
			);
			await sendMessage.mutateAsync({
				sessionId: targetSessionId,
				content: "Enviei estes anexos para complementar a descrição.",
				attachmentIds: attachments.map((attachment) => attachment.id),
			});
		}
        setIdempotencyKey(crypto.randomUUID());
      }
      setDescription("");
		setFiles([]);
    } catch {
      setError("Não foi possível enviar agora. Tente novamente.");
    }
  }

  function startNewSession() {
    setSelectedSessionId(null);
    setDescription("");
    setError(null);
	setFiles([]);
  }

	function chooseFiles(event: ChangeEvent<HTMLInputElement>) {
		const next = Array.from(event.target.files ?? []).slice(0, 5);
		setFiles(next);
		event.target.value = "";
	}

  const loadError = sessionsQuery.isError
    ? "Não foi possível carregar seus atendimentos."
    : messagesQuery.isError
      ? "Não foi possível carregar as mensagens deste atendimento."
      : null;

  return (
    <main className="mx-auto grid min-h-svh w-full max-w-5xl gap-6 p-4 md:grid-cols-[18rem_minmax(0,1fr)] md:p-8">
      <aside className="rounded-xl border bg-card p-4 text-card-foreground">
        <h1 className="text-title-lg font-semibold">Suporte dos Sistemas Innova</h1>
        <p className="mt-2 text-body text-muted-foreground">
          O Concierge responde dúvidas e coleta o contexto. Mudanças técnicas
          só avançam com aprovação.
        </p>
        <button
          type="button"
          onClick={startNewSession}
          className="mt-5 w-full rounded-md bg-primary px-4 py-2 text-body font-medium text-primary-foreground disabled:opacity-50"
        >
          Nova dúvida ou problema
        </button>

        <div className="mt-6">
          <h2 className="text-caption font-medium uppercase tracking-wide text-muted-foreground">
            Seus atendimentos
          </h2>
          {sessionsQuery.isPending ? (
            <p className="mt-3 text-body text-muted-foreground">Carregando…</p>
          ) : sessions.length === 0 ? (
            <p className="mt-3 text-body text-muted-foreground">
              Nenhum atendimento ainda.
            </p>
          ) : (
            <ul className="mt-3 space-y-2">
              {sessions.map((session) => {
                const selected =
                  session.session_id === selectedSession?.session_id;
                return (
                  <li key={session.id}>
                    <button
                      type="button"
                      aria-current={selected ? "page" : undefined}
                      onClick={() => {
                        setSelectedSessionId(session.session_id);
                        setError(null);
                      }}
                      className="w-full rounded-md border px-3 py-2 text-left transition-colors hover:bg-muted aria-[current=page]:border-primary aria-[current=page]:bg-muted"
                    >
                      <span className="block text-body font-medium">
                        {session.public_code}
                      </span>
                      <span className="mt-1 block text-caption text-muted-foreground">
                        {supportStateLabel[session.state] ??
                          "Em acompanhamento"}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </aside>

      <section className="flex min-h-[32rem] min-w-0 flex-col rounded-xl border bg-card p-4 text-card-foreground md:p-6">
        <div className="border-b pb-4">
          <h2 className="text-title font-semibold">
            {selectedSession
              ? selectedSession.public_code
              : "Como podemos ajudar?"}
          </h2>
          <p className="mt-1 text-body text-muted-foreground">
            {selectedSession
              ? (supportStateLabel[selectedSession.state] ??
                "Em acompanhamento")
              : "Conte o que você quer fazer ou o que não funcionou. Vamos registrar o contexto para análise."}
          </p>
        </div>

        {(error || loadError) && (
          <p
            role="alert"
            className="mt-4 rounded-md bg-destructive/10 p-3 text-body text-destructive"
          >
            {error ?? loadError}
          </p>
        )}

        <div className="flex-1 space-y-3 overflow-y-auto py-5">
          {selectedSession && messagesQuery.isPending ? (
            <p className="text-body text-muted-foreground">
              Carregando mensagens…
            </p>
          ) : selectedSession && messages.length === 0 ? (
            <p className="text-body text-muted-foreground">
              Nenhuma mensagem encontrada.
            </p>
          ) : (
            messages.map((message) => (
              <article
                key={message.id}
                className={`max-w-[85%] rounded-lg border p-3 text-body ${
                  message.role === "user"
                    ? "ml-auto bg-primary/5"
                    : "mr-auto bg-muted"
                }`}
              >
                <p className="mb-1 text-caption font-medium text-muted-foreground">
                  {message.role === "user" ? "Você" : "Suporte"}
                </p>
                <p className="whitespace-pre-wrap break-words">
                  {message.content}
                </p>
				{message.attachments && message.attachments.length > 0 && (
					<ul className="mt-3 space-y-2">
						{message.attachments.map((attachment) => (
							<li key={attachment.id}>
								<a className="underline" href={attachment.download_url} target="_blank" rel="noreferrer">
									{attachment.filename}
								</a>
							</li>
						))}
					</ul>
				)}
              </article>
            ))
          )}
        </div>

        {selectedSession &&
          ["resposta_proposta", "aguardando_confirmacao"].includes(
            selectedSession.state,
          ) && (
            <div className="flex flex-wrap gap-3 border-t pt-4">
              <button
                type="button"
                disabled={resolutionFeedback.isPending}
                onClick={() =>
                  resolutionFeedback.mutate({
                    caseId: selectedSession.id,
                    resolved: true,
                  })
                }
                className="rounded-md bg-primary px-4 py-2 text-body font-medium text-primary-foreground disabled:opacity-50"
              >
                Resolveu
              </button>
              <button
                type="button"
                disabled={resolutionFeedback.isPending}
                onClick={() =>
                  resolutionFeedback.mutate({
                    caseId: selectedSession.id,
                    resolved: false,
                  })
                }
                className="rounded-md border px-4 py-2 text-body font-medium disabled:opacity-50"
              >
                Ainda não resolveu
              </button>
            </div>
          )}

        <form onSubmit={submit} className="grid gap-3 border-t pt-4">
          <label htmlFor="description" className="text-body font-medium">
            {selectedSession
              ? "Nova mensagem"
              : "Descreva sua dúvida ou problema"}
          </label>
          <textarea
            id="description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            maxLength={8192}
            required
            rows={5}
            placeholder="Explique o que aconteceu, em qual tela e o que você esperava ver."
            className="w-full resize-y rounded-md border bg-background p-3 text-body outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
		  <div className="flex flex-wrap items-center gap-3">
			<label className="cursor-pointer rounded-md border px-3 py-2 text-body">
				Adicionar print ou PDF
				<input className="sr-only" type="file" accept="image/png,image/jpeg,image/webp,application/pdf" multiple onChange={chooseFiles} />
			</label>
			{files.length > 0 && (
				<span className="text-caption text-muted-foreground">{files.length} arquivo(s) selecionado(s)</span>
			)}
		  </div>
          <button
            type="submit"
            disabled={submitting || description.trim().length === 0}
            className="justify-self-end rounded-md bg-primary px-5 py-2 text-body font-medium text-primary-foreground disabled:cursor-not-allowed disabled:opacity-50"
          >
            {submitting ? "Concierge analisando…" : "Enviar para o Concierge"}
          </button>
        </form>
      </section>
    </main>
  );
}
