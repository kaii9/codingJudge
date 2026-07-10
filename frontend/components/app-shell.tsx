"use client";

import { LogIn, LogOut, Server, UserPlus } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from "react";
import { getCurrentUser, login, logout, register } from "@/lib/api";
import type { AuthInput, User } from "@/lib/types";

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

function isUser(value: unknown): value is User {
  return (
    typeof value === "object"
    && value !== null
    && "id" in value
    && "username" in value
    && typeof value.id === "string"
    && typeof value.username === "string"
  );
}

export function AppShell({ children }: AppShellProps) {
  const [serviceState, setServiceState] = useState<ServiceState>("checking");
  const [user, setUser] = useState<User | null>(null);
  const [authInput, setAuthInput] = useState<AuthInput>({ username: "", password: "" });
  const [authPending, setAuthPending] = useState(false);
  const [authError, setAuthError] = useState<string | null>(null);

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

  useEffect(() => {
    let active = true;

    void getCurrentUser().then(currentUser => {
      if (active && isUser(currentUser)) {
        setUser(currentUser);
      }
    }).catch(() => {
      if (active) setUser(null);
    });

    return () => {
      active = false;
    };
  }, []);

  const authenticate = useCallback(async (mode: "login" | "register") => {
    setAuthPending(true);
    setAuthError(null);
    try {
      const nextUser = mode === "login"
        ? await login(authInput)
        : await register(authInput);
      setUser(nextUser);
      setAuthInput({ username: "", password: "" });
    } catch {
      setAuthError(mode === "login" ? "Login failed" : "Register failed");
    } finally {
      setAuthPending(false);
    }
  }, [authInput]);

  const handleAuthSubmit = useCallback((event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    void authenticate("login");
  }, [authenticate]);

  const handleLogout = useCallback(async () => {
    setAuthPending(true);
    setAuthError(null);
    try {
      await logout();
      setUser(null);
    } catch {
      setAuthError("Logout failed");
    } finally {
      setAuthPending(false);
    }
  }, []);

  const serviceLabel = serviceLabels[serviceState];

  return (
    <div className="app-shell">
      <header className="app-shell__topbar">
        <div className="app-shell__topbar-inner">
          <Link className="app-shell__brand" href="/">
            GOJUDGE
          </Link>
          <nav className="app-shell__nav" aria-label="Primary navigation">
            <Link href="/">Problems</Link>
            <Link href="/submissions">Submissions</Link>
            <Link href="/leaderboard">Leaderboard</Link>
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
          <div className="auth-panel" aria-live="polite">
            {user ? (
              <div className="auth-panel__user">
                <span className="auth-panel__name">{user.username}</span>
                <button type="button" onClick={handleLogout} disabled={authPending}>
                  <LogOut size={14} aria-hidden="true" />
                  <span>Log out</span>
                </button>
              </div>
            ) : (
              <form className="auth-panel__form" onSubmit={handleAuthSubmit}>
                <input
                  aria-label="Username"
                  autoComplete="username"
                  value={authInput.username}
                  onChange={event => setAuthInput(current => ({
                    ...current,
                    username: event.target.value,
                  }))}
                  placeholder="username"
                />
                <input
                  aria-label="Password"
                  autoComplete="current-password"
                  type="password"
                  value={authInput.password}
                  onChange={event => setAuthInput(current => ({
                    ...current,
                    password: event.target.value,
                  }))}
                  placeholder="password"
                />
                <button type="submit" disabled={authPending}>
                  <LogIn size={14} aria-hidden="true" />
                  <span>Log in</span>
                </button>
                <button type="button" disabled={authPending} onClick={() => void authenticate("register")}>
                  <UserPlus size={14} aria-hidden="true" />
                  <span>Register</span>
                </button>
              </form>
            )}
            {authError ? <span className="auth-panel__error" role="alert">{authError}</span> : null}
          </div>
        </div>
      </header>
      <div className="app-shell__content">{children}</div>
    </div>
  );
}
