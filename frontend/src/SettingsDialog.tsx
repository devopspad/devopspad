import { useEffect, useState } from "react";
import { LockKeyhole, Sparkles } from "lucide-react";
import { api, desktop, errorText } from "./api";
import { Modal } from "./Modal";
import type { AISettings } from "./types";
export function SettingsDialog({
  onClose,
  recovery,
}: {
  onClose: () => void;
  recovery: {
    enabled: boolean;
    error: string;
    changeEnabled: (enabled: boolean) => void;
    retry: () => void;
  };
}) {
  const [settings, set] = useState<AISettings>({
    endpoint: "https://api.openai.com/v1",
    model: "",
    apiKey: "",
    hasKey: false,
  });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (desktop)
      api
        .GetAISettings()
        .then(set)
        .catch((e) => setError(errorText(e)));
  }, []);
  return (
    <Modal
      title="Settings"
      subtitle="Manage local drafts and your AI provider."
      onClose={onClose}
    >
      <div className="form-body recovery-settings">
        <h3>Workspace recovery</h3>
        <label className="recovery-toggle">
          <input
            type="checkbox"
            checked={recovery.enabled}
            onChange={(e) => recovery.changeEnabled(e.target.checked)}
          />
          Restore query drafts on startup
        </label>
        <p className="field-help">
          Saves query text and database/context scopes on this device, without
          encryption. Results and connection credentials are excluded. Query
          text may contain sensitive values. Turning this off deletes the saved
          recovery copy. Open tabs stay available.
        </p>
        {recovery.error && <div className="error-box">{recovery.error}</div>}
        {recovery.enabled && (
          <button className="button" type="button" onClick={recovery.retry}>
            Save recovery copy now
          </button>
        )}
        {recovery.error && (
          <button
            className="button"
            type="button"
            onClick={() => recovery.changeEnabled(false)}
          >
            Turn off recovery and clear saved copy
          </button>
        )}
      </div>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          try {
            await api.SaveAISettings(settings);
            onClose();
          } catch (err) {
            setError(errorText(err));
          } finally {
            setBusy(false);
          }
        }}
      >
        <div className="form-body">
          <div className="settings-intro">
            <Sparkles size={23} />
            <div>
              <strong>Your copilot, your choice.</strong>
              <p>
                Connect an OpenAI-compatible API or a local provider such as
                Ollama.
              </p>
            </div>
          </div>
          <label>
            API base URL
            <input
              required
              type="url"
              value={settings.endpoint}
              onChange={(e) => set({ ...settings, endpoint: e.target.value })}
              placeholder="http://localhost:11434/v1"
            />
          </label>
          <label>
            Model
            <input
              required
              value={settings.model}
              onChange={(e) => set({ ...settings, model: e.target.value })}
              placeholder="Enter your provider’s model ID"
            />
          </label>
          <label>
            API key
            <input
              type="password"
              autoComplete="new-password"
              value={settings.apiKey || ""}
              onChange={(e) => set({ ...settings, apiKey: e.target.value })}
              placeholder={
                settings.hasKey
                  ? "Saved in keychain · leave blank to keep"
                  : "Optional for local providers"
              }
            />
          </label>
          <div className="notice">
            <LockKeyhole size={16} />
            <span>
              Only your prompt is sent by default. Resource names are shared
              only when you enable context. Credentials and query results are
              never added automatically.
            </span>
          </div>
          {!desktop && (
            <p className="field-help">
              Provider settings are available in the desktop app.
            </p>
          )}
          {error && (
            <div className="error-box" role="alert">
              {error}
            </div>
          )}
        </div>
        <div className="modal-footer">
          <button type="button" className="button" onClick={onClose}>
            Cancel
          </button>
          <button className="button primary" disabled={busy || !desktop}>
            {busy ? "Saving…" : "Save provider"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
