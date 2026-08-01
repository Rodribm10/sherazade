import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { ReporterRouteGate } from "./reporter-route-gate";

const replace = vi.fn();
let pathname = "/innova/issues";

vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
  useRouter: () => ({ replace }),
}));

afterEach(() => {
  cleanup();
  replace.mockReset();
  pathname = "/innova/issues";
});

describe("ReporterRouteGate", () => {
  it("redireciona reporter sem montar a superfície técnica", async () => {
    render(
      <ReporterRouteGate
        role="reporter"
        workspaceSlug="innova"
        loadingIndicator={<div>Redirecionando</div>}
      >
        <div>Dashboard técnico</div>
      </ReporterRouteGate>,
    );

    expect(screen.getByText("Redirecionando")).toBeInTheDocument();
    expect(screen.queryByText("Dashboard técnico")).not.toBeInTheDocument();
    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith("/innova/support"),
    );
  });

  it("mantém reporter dentro da superfície de suporte", () => {
    pathname = "/innova/support";
    render(
      <ReporterRouteGate
        role="reporter"
        workspaceSlug="innova"
        loadingIndicator={<div>Redirecionando</div>}
      >
        <div>Central de suporte</div>
      </ReporterRouteGate>,
    );

    expect(screen.getByText("Central de suporte")).toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });

  it("não altera a navegação de membros técnicos", () => {
    render(
      <ReporterRouteGate
        role="member"
        workspaceSlug="innova"
        loadingIndicator={<div>Redirecionando</div>}
      >
        <div>Dashboard técnico</div>
      </ReporterRouteGate>,
    );

    expect(screen.getByText("Dashboard técnico")).toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });
});
