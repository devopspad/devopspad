import { useEffect, useState } from "react";
import { api, errorText } from "./api";
import { ConfigCodeEditor } from "./ConfigCodeEditor";
import type { OpsNginxConfig as NginxConfig } from "./ops-api";

// Keep unsaved text in memory when navigating between applications/environments.
const drafts = new Map<string, { base: NginxConfig; text: string }>();

export function OpsNginxConfig({
  projectRoot,
  environment,
  application,
  path,
  saving,
  onBusy,
}: {
  projectRoot: string;
  environment: string;
  application: string;
  path?: string;
  saving: boolean;
  onBusy: (busy: boolean) => void;
}) {
  const scope = JSON.stringify([projectRoot, environment, application, path || ""]);
  const [config, setConfig] = useState<NginxConfig>();
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(false);
  const [version, setVersion] = useState(0);
  useEffect(() => {
    let stale = false;
    setConfig(undefined);
    setError("");
    setNotice("");
    setEditing(false);
    if (!path) {
      setLoading(false);
      return;
    }
    setLoading(true);
    api
      .ReadOpsNginx(environment, application)
      .then((value) => {
        if (!stale) {
          const saved = drafts.get(scope);
          setConfig(saved?.base || value);
          setDraft(saved?.text ?? value.text);
          setEditing(!!saved);
          if (saved && saved.base.revision !== value.revision)
            setError(
              "The config on the host changed. Your draft is kept; copy what you need, cancel editing and refresh.",
            );
        }
      })
      .catch((e) => {
        if (!stale) setError(errorText(e));
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [environment, application, path, version, scope]);
  async function save() {
    if (!config) return;
    onBusy(true);
    setError("");
    setNotice("");
    try {
      const updated = await api.SaveOpsNginx(environment, application, {
        revision: config.revision,
        text: draft,
      });
      drafts.delete(scope);
      setConfig(updated);
      setDraft(updated.text);
      setEditing(false);
      setNotice(updated.message || "Nginx config saved and reloaded");
    } catch (e) {
      setError(errorText(e));
    } finally {
      onBusy(false);
    }
  }
  return (
    <section className="ops-config-card">
      <h3>Nginx · host</h3>
      {!path ? (
        <p className="ops-muted">
          Set nginxPath on the application in Project mapping, e.g.
          /etc/nginx/conf.d/app.conf. It is read over the selected environment’s SSH connection.
        </p>
      ) : (
        <>
          <p>
            <code>{path}</code>
          </p>
          <p className="ops-muted">
            Saving checks the file revision, runs nginx -t and reloads. If the test fails, the previous config is restored.
          </p>
          {error && (
            <div className="ops-error" role="alert">
              {error}
            </div>
          )}
          {notice && (
            <div className="ops-notice" role="status">
              {notice}
            </div>
          )}
          <div className="ops-inline">
            <button
              className="ops-btn"
              disabled={saving || loading || editing}
              onClick={() => setVersion((v) => v + 1)}
            >
              Refresh Nginx
            </button>
            {config &&
              (editing ? (
                <>
                  <button
                    className="ops-btn"
                    disabled={saving || draft === config.text}
                    onClick={() => void save()}
                  >
                    Save and reload Nginx
                  </button>
                  <button
                    className="ops-btn"
                    disabled={saving}
                    onClick={() => {
                      drafts.delete(scope);
                      setDraft(config.text);
                      setEditing(false);
                      setError("");
                      setVersion((v) => v + 1);
                    }}
                  >
                    Cancel
                  </button>
                </>
              ) : (
                <button
                  className="ops-btn"
                  disabled={saving}
                  onClick={() => {
                    setEditing(true);
                    setNotice("");
                  }}
                >
                  Edit Nginx
                </button>
              ))}
          </div>
          {loading && <p role="status">Loading host Nginx config…</p>}
          {config && (
            <ConfigCodeEditor
              filename="nginx.conf"
              label="Nginx config content"
              value={editing ? draft : config.text}
              editable={editing && !saving}
              onChange={(value) => {
                setDraft(value);
                if (editing) drafts.set(scope, { base: config, text: value });
              }}
            />
          )}
        </>
      )}
    </section>
  );
}
