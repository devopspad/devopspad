import { useEffect, useRef, useState } from "react";
import type { Tab } from "./types";

const storageKey = "devopspad-workspace-v1";
const maxSize = 2 * 1024 * 1024;
type Snapshot = { tabs: Tab[]; activeId: string; selected: string };
type Session = { version: 2; enabled: boolean; workspace: Snapshot };

// Only query drafts and scopes are saved, never results or connection credentials.
function draft(tab: Tab): Tab {
  return {
    id: tab.id,
    connectionId: tab.connectionId,
    title: tab.title,
    query: tab.query,
    database: tab.database,
    context: tab.context,
  };
}
function snapshot(value: unknown): Snapshot {
  if (!value || typeof value !== "object") throw new Error("Invalid workspace");
  const s = value as Snapshot;
  if (
    !Array.isArray(s.tabs) ||
    s.tabs.length > 100 ||
    typeof s.activeId !== "string" ||
    typeof s.selected !== "string"
  )
    throw new Error("Invalid workspace");
  const ids = new Set<string>();
  const tabs = s.tabs.map((t) => {
    if (
      !t ||
      typeof t.id !== "string" ||
      !t.id ||
      ids.has(t.id) ||
      typeof t.connectionId !== "string" ||
      typeof t.title !== "string" ||
      typeof t.query !== "string" ||
      (t.database !== undefined && typeof t.database !== "string") ||
      (t.context !== undefined && typeof t.context !== "string")
    )
      throw new Error("Invalid query draft");
    ids.add(t.id);
    return draft(t);
  });
  return {
    tabs,
    activeId: ids.has(s.activeId) ? s.activeId : tabs[0]?.id || "",
    selected: s.selected,
  };
}
export function loadSession(): { session: Session; error: string } {
  const fallback: Session = {
    version: 2,
    enabled: false,
    workspace: { tabs: [], activeId: "", selected: "" },
  };
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) return { session: fallback, error: "" };
    if (raw.length > maxSize) throw new Error("Workspace too large");
    const s = JSON.parse(raw);
    if (![1, 2].includes(s.version) || s.enabled !== true)
      throw new Error("Unsupported workspace");
    // Migrate only real connection drafts from the old two-workspace format.
    const restored = snapshot(
      s.version === 1 ? s.workspaces?.personal : s.workspace,
    );
    return {
      session: { version: 2, enabled: true, workspace: restored },
      error: "",
    };
  } catch {
    return {
      session: fallback,
      error:
        "Saved drafts could not be restored. Recovery is off; the saved copy has not been overwritten.",
    };
  }
}
export function useWorkspaceSession(
  initial: ReturnType<typeof loadSession>,
  current: Snapshot,
) {
  const [enabled, setEnabled] = useState(initial.session.enabled);
  const [error, setError] = useState(initial.error);
  const currentRef = useRef(current);
  currentRef.current = current;
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;
  const save = () => {
    if (!enabledRef.current) return;
    try {
      const now = currentRef.current;
      if (now.tabs.length > 100) throw new Error("Too many drafts");
      const raw = JSON.stringify({
        version: 2,
        enabled: true,
        workspace: { ...now, tabs: now.tabs.map(draft) },
      });
      if (raw.length > maxSize) throw new Error("Drafts too large");
      localStorage.setItem(storageKey, raw);
      setError("");
    } catch {
      setError(
        "Draft recovery could not be saved. Keep this window open and save important queries. Close unused tabs or free local storage, then retry.",
      );
    }
  };
  const saveRef = useRef(save);
  saveRef.current = save;
  useEffect(() => {
    const timer = setTimeout(() => saveRef.current(), 300);
    return () => clearTimeout(timer);
  }, [enabled, current.tabs, current.activeId, current.selected]);
  useEffect(() => {
    const flush = () => saveRef.current();
    const visibility = () => {
      if (document.visibilityState === "hidden") flush();
    };
    window.addEventListener("pagehide", flush);
    window.addEventListener("beforeunload", flush);
    document.addEventListener("visibilitychange", visibility);
    return () => {
      window.removeEventListener("pagehide", flush);
      window.removeEventListener("beforeunload", flush);
      document.removeEventListener("visibilitychange", visibility);
    };
  }, []);
  function changeEnabled(next: boolean) {
    if (!next) {
      try {
        localStorage.removeItem(storageKey);
      } catch {
        setError(
          "Saved drafts could not be removed. Local storage is unavailable; retry when it is accessible.",
        );
        return;
      }
    }
    enabledRef.current = next;
    setEnabled(next);
    setError("");
    if (next) save();
  }
  function workspace(validConnections: string[]) {
    const s = currentRef.current;
    const tabs = s.tabs.filter((t) =>
      validConnections.includes(t.connectionId),
    );
    return {
      tabs,
      activeId: tabs.some((t) => t.id === s.activeId)
        ? s.activeId
        : tabs[0]?.id || "",
      selected: validConnections.includes(s.selected)
        ? s.selected
        : tabs[0]?.connectionId || validConnections[0] || "",
    };
  }
  return { enabled, error, changeEnabled, workspace, retry: save };
}
