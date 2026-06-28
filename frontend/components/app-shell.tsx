"use client";

import { Server } from "lucide-react";
import Link from "next/link";
import { useEffect, useState, type ReactNode } from "react";

type ServiceState = "checking" | "online" | "unavailable";
const HEALTH_CHECK_TIMEOUT_MS = 5_000;

const serviceLabels: Record<ServiceState, string> = {
  checking: "Checking",
  online: "Online",
  unavailable: "Unavailable",
};

interface AppShellProps {
  children: ReactNode;
}

function isHealthyResponse(value: unknown): value is { status: "ok" } {
  return typeof value === "object" && value !== null && "status" in value && value.status === "ok";
}

export function AppShell({ children }: AppShellProps) {
  const [serviceState, setServiceState] = useState<ServiceState>("checking");

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    let timedOut = false;
    const deadlineId = globalThis.setTimeout(() => {
      timedOut = true;
      controller.abort();

      if (active) {
        setServiceState("unavailable");
      }
    }, HEALTH_CHECK_TIMEOUT_MS);

    async function checkHealth() {
      try {
        const response = await fetch("/api/healthz", { signal: controller.signal });
        const payload: unknown = response.ok ? await response.json() : null;

        if (active && !timedOut) {
          setServiceState(response.ok && isHealthyResponse(payload) ? "online" : "unavailable");
        }
      } catch {
        if (active && !timedOut) {
          setServiceState("unavailable");
        }
      } finally {
        globalThis.clearTimeout(deadlineId);
      }
    }

    void checkHealth();

    return () => {
      active = false;
      globalThis.clearTimeout(deadlineId);
      controller.abort();
    };
  }, []);

  const serviceLabel = serviceLabels[serviceState];

  return (
    <div className="app-shell">
      <header className="app-shell__topbar">
        <div className="app-shell__topbar-inner app-shell__topbar-inner--responsive">
          <Link className="app-shell__brand" href="/">
            GOJUDGE
          </Link>
          <nav className="app-shell__nav" aria-label="Primary navigation">
            <Link href="/">Problems</Link>
            <Link href="/submissions">Submissions</Link>
          </nav>
          <div
            className="service-indicator"
            data-state={serviceState}
            role="status"
            aria-label={`Service status: ${serviceLabel}`}
            aria-live="polite"
          >
            <Server aria-hidden="true" focusable="false" size={14} strokeWidth={2.25} />
            <span className="service-indicator__dot" aria-hidden="true" />
            <span>{serviceLabel}</span>
          </div>
        </div>
      </header>
      <div className="app-shell__content">{children}</div>
    </div>
  );
}
