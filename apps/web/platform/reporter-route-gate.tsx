"use client";

import type { ReactNode } from "react";
import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import type { MemberRole } from "@multica/core/types";

export function ReporterRouteGate({
  children,
  loadingIndicator,
  role,
  workspaceSlug,
}: {
  children: ReactNode;
  loadingIndicator: ReactNode;
  role: MemberRole | undefined;
  workspaceSlug: string;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const supportPath = `/${workspaceSlug}/support`;
  const isSupportPath =
    pathname === supportPath || pathname.startsWith(`${supportPath}/`);
  const mustRedirect = role === "reporter" && !isSupportPath;

  useEffect(() => {
    if (mustRedirect) router.replace(supportPath);
  }, [mustRedirect, router, supportPath]);

  if (mustRedirect) return loadingIndicator;
  return children;
}
