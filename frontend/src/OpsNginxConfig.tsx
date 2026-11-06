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
              "源站配置已变化，草稿已保留；请复制所需修改后取消编辑并刷新。",
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
      setNotice(updated.message || "Nginx 配置已保存并重载");
    } catch (e) {
      setError(errorText(e));
    } finally {
      onBusy(false);
    }
  }
  return (
    <section className="ops-config-card">
      <h3>Nginx · 宿主机</h3>
      {!path ? (
        <p className="ops-muted">
          在项目映射的应用中设置 nginxPath，例如
          /etc/nginx/conf.d/app.conf；通过所选环境的 SSH 连接读取。
        </p>
      ) : (
        <>
          <p>
            <code>{path}</code>
          </p>
          <p className="ops-muted">
            保存时校验当前文件版本，执行 nginx -t 后重载；校验失败会恢复原配置。
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
              刷新 Nginx
            </button>
            {config &&
              (editing ? (
                <>
                  <button
                    className="ops-btn"
                    disabled={saving || draft === config.text}
                    onClick={() => void save()}
                  >
                    保存并重载 Nginx
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
                    取消
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
                  编辑 Nginx
                </button>
              ))}
          </div>
          {loading && <p role="status">正在读取宿主机 Nginx 配置…</p>}
          {config && (
            <ConfigCodeEditor
              filename="nginx.conf"
              label="Nginx 配置内容"
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
