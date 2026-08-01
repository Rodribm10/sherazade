import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api";
import SupportPage from "./page";

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "workspace-1",
}));

const apiStub = {
  createSupportSession: vi.fn(),
  listSupportMessages: vi.fn(),
  listSupportSessions: vi.fn(),
  sendSupportMessage: vi.fn(),
};

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <SupportPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  apiStub.createSupportSession.mockReset();
  apiStub.listSupportMessages.mockReset();
  apiStub.listSupportSessions.mockReset();
  apiStub.sendSupportMessage.mockReset();
  apiStub.listSupportSessions.mockResolvedValue([]);
  apiStub.listSupportMessages.mockResolvedValue([]);
  setApiInstance(apiStub as unknown as ApiClient);
});

afterEach(() => {
  cleanup();
});

describe("SupportPage", () => {
  it("cria um atendimento sem permitir seleção de agente", async () => {
    apiStub.createSupportSession.mockResolvedValue({
      id: "case-10",
      public_code: "SUP-000010",
      session_id: "session-10",
      app_key: "inaudit",
      state: "novo",
    });
    apiStub.listSupportMessages.mockResolvedValue([
      {
        id: "message-10",
        role: "user",
        content: "Minha pontuação não apareceu",
        created_at: "2026-08-01T18:00:00Z",
      },
    ]);
    const user = userEvent.setup();
    renderPage();

    await user.type(
      screen.getByLabelText("Descreva sua dúvida ou problema"),
      "Minha pontuação não apareceu",
    );
    await user.click(
      screen.getByRole("button", { name: "Enviar para suporte" }),
    );

    await waitFor(() =>
      expect(apiStub.createSupportSession).toHaveBeenCalledTimes(1),
    );
    expect(apiStub.createSupportSession.mock.calls[0]?.[0]).toMatchObject({
      description: "Minha pontuação não apareceu",
    });
    expect(apiStub.createSupportSession.mock.calls[0]?.[0]).not.toHaveProperty(
      "agent_id",
    );
    expect(await screen.findAllByText("SUP-000010")).not.toHaveLength(0);
  });

  it("abre o histórico próprio e envia uma mensagem de acompanhamento", async () => {
    apiStub.listSupportSessions.mockResolvedValue([
      {
        id: "case-1",
        public_code: "SUP-000001",
        session_id: "session-1",
        app_key: "inaudit",
        state: "novo",
      },
    ]);
    apiStub.listSupportMessages.mockResolvedValue([
      {
        id: "message-1",
        role: "user",
        content: "Primeira mensagem",
        created_at: "2026-08-01T18:00:00Z",
      },
    ]);
    apiStub.sendSupportMessage.mockResolvedValue({
      id: "message-2",
      role: "user",
      content: "Informação adicional",
      created_at: "2026-08-01T18:01:00Z",
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole("button", { name: /SUP-000001/ }));
    expect(await screen.findByText("Primeira mensagem")).toBeInTheDocument();
    await user.type(
      screen.getByLabelText("Nova mensagem"),
      "Informação adicional",
    );
    await user.click(
      screen.getByRole("button", { name: "Enviar para suporte" }),
    );

    await waitFor(() =>
      expect(apiStub.sendSupportMessage).toHaveBeenCalledWith(
        "session-1",
        "Informação adicional",
      ),
    );
    expect(await screen.findByText("Informação adicional")).toBeInTheDocument();
  });

  it("exibe erro de envio sem apagar o relato", async () => {
    apiStub.createSupportSession.mockRejectedValue(new Error("offline"));
    const user = userEvent.setup();
    renderPage();

    const field = screen.getByLabelText("Descreva sua dúvida ou problema");
    await user.type(field, "Não consigo abrir a tela");
    await user.click(
      screen.getByRole("button", { name: "Enviar para suporte" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Não foi possível enviar agora. Tente novamente.",
    );
    expect(field).toHaveValue("Não consigo abrir a tela");
  });
});
