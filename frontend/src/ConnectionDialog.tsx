import { useEffect, useState } from "react";
import {
  Check,
  FolderOpen,
  LoaderCircle,
  LockKeyhole,
  PlugZap,
} from "lucide-react";
import { SSHFields } from "./SSHFields";
import { Modal } from "./Modal";
import { api, desktop, errorText } from "./api";
import {
  engines,
  type Connection,
  type Kind,
  type KubeconfigInfo,
} from "./types";
export function EngineIcon({
  kind,
  small = false,
}: {
  kind: Kind;
  small?: boolean;
}) {
  const e = engines[kind];
  return (
    <span
      className={`engine-icon ${small ? "small" : ""}`}
      style={{ color: e.color, background: `${e.color}12` }}
    >
      {e.short}
    </span>
  );
}
export function ConnectionDialog({
  initial,
  defaultKind = "mysql",
  onClose,
  onSaved,
}: {
  initial?: Connection;
  defaultKind?: Kind;
  onClose: () => void;
  onSaved: (c: Connection) => void;
}) {
  const [c, set] = useState<Connection>(
    initial || {
      id: "",
      name: "",
      kind: defaultKind,
      host: "localhost",
      port: engines[defaultKind].port,
      username: "",
      password: "",
      database: defaultKind === "k3s" || defaultKind === "k8s" ? "default" : "",
      environment: "Development",
      tls: false,
      kubeconfig: "",
      context: "",
      hasPassword: false,
    },
  );
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const patch = (v: Partial<Connection>) => {
    set((x) => ({ ...x, ...v }));
    setSuccess("");
    setError("");
  };
  const cluster = c.kind === "k8s" || c.kind === "k3s";
  const [kubeInfo, setKubeInfo] = useState<KubeconfigInfo>();
  const [kubeError, setKubeError] = useState("");
  const [readingKube, setReadingKube] = useState(false);
  useEffect(() => {
    setKubeInfo(undefined);
    setReadingKube(false);
    setKubeError("");
    if (!cluster || !c.kubeconfig || !desktop) return;
    let stale = false;
    setReadingKube(true);
    const timer = window.setTimeout(async () => {
      try {
        const info = await api.ReadKubeconfig(c.kubeconfig);
        if (stale) return;
        setKubeInfo(info);
        set((current) => {
          if (current.context) return current;
          const selected = info.contexts.find(
            (x) => x.name === info.currentContext,
          );
          return {
            ...current,
            context: info.currentContext,
            database: initial
              ? current.database
              : selected?.namespace || current.database,
          };
        });
      } catch (e) {
        if (!stale) setKubeError(errorText(e));
      } finally {
        if (!stale) setReadingKube(false);
      }
    }, 250);
    return () => {
      stale = true;
      window.clearTimeout(timer);
    };
  }, [c.kubeconfig, cluster]);
  async function action(test: boolean) {
    setBusy(test ? "test" : "save");
    setError("");
    setSuccess("");
    try {
      if (test)
        setSuccess(`Connection verified in ${await api.TestConnection(c)} ms`);
      else onSaved(await api.SaveConnection(c));
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy("");
    }
  }
  return (
    <Modal
      title={initial ? "Edit connection" : "New connection"}
      subtitle="Your infrastructure, one workspace."
      onClose={onClose}
      wide
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void action(false);
        }}
      >
        <div className="engine-picker">
          {(Object.keys(engines) as Kind[]).map((kind) => (
            <button
              type="button"
              key={kind}
              className={c.kind === kind ? "chosen" : ""}
              onClick={() =>
                patch({
                  kind,
                  port: engines[kind].port,
                  database:
                    kind === "redis"
                      ? "0"
                      : kind === "k8s" || kind === "k3s"
                        ? "default"
                        : "",
                  username: "",
                })
              }
            >
              <EngineIcon kind={kind} />
              <span>{engines[kind].name}</span>
            </button>
          ))}
        </div>
        <div className="form-body">
          <div className="form-grid">
            <label>
              Connection name
              <input
                required
                placeholder="e.g. Production database"
                value={c.name}
                onChange={(e) => patch({ name: e.target.value })}
              />
            </label>
            <label>
              Environment
              <input
                list="connection-environments"
                value={c.environment}
                onChange={(e) => patch({ environment: e.target.value })}
                placeholder="例如 Production、heartie-pro"
              />
              <datalist id="connection-environments">
                {["Development", "Staging", "Production"].map((name) => (
                  <option key={name} value={name} />
                ))}
              </datalist>
            </label>
          </div>
          {cluster ? (
            <>
              <label>
                Kubeconfig file
                <div className="input-action">
                  <input
                    required
                    placeholder="/Users/you/.kube/config"
                    value={c.kubeconfig}
                    onChange={(e) => patch({ kubeconfig: e.target.value })}
                  />
                  <button
                    type="button"
                    className="icon-button"
                    aria-label="Choose kubeconfig"
                    onClick={async () => {
                      try {
                        const path = await api.SelectKubeconfig();
                        if (path) patch({ kubeconfig: path, context: "" });
                      } catch (e) {
                        setError(errorText(e));
                      }
                    }}
                  >
                    <FolderOpen size={17} />
                  </button>
                </div>
              </label>
              <div className="form-grid">
                <label>
                  Context
                  <select
                    aria-label="Context"
                    value={c.context}
                    onChange={(e) => {
                      const selected = kubeInfo?.contexts.find(
                        (x) => x.name === e.target.value,
                      );
                      patch({
                        context: e.target.value,
                        database: selected?.namespace || "default",
                      });
                    }}
                  >
                    <option value="">Current context</option>
                    {c.context &&
                      !kubeInfo?.contexts.some((x) => x.name === c.context) && (
                        <option value={c.context}>{c.context}</option>
                      )}
                    {kubeInfo?.contexts.map((x) => (
                      <option key={x.name} value={x.name}>
                        {x.name} · {x.cluster}
                        {x.execAuth ? " (exec auth unsupported)" : ""}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Namespace
                  <input
                    placeholder="default (or * for all)"
                    value={c.database}
                    onChange={(e) => patch({ database: e.target.value })}
                  />
                </label>
              </div>
              {readingKube && (
                <p className="field-help">Reading kubeconfig contexts…</p>
              )}
              {kubeError && (
                <p className="error-box" role="alert">
                  {kubeError}
                </p>
              )}
              <p className="field-help">
                Token and client certificate authentication are supported.
                Exec-based authentication is not available yet.
              </p>
            </>
          ) : (
            <>
              <div className="form-grid host-grid">
                <label>
                  Host
                  <input
                    required
                    value={c.host}
                    onChange={(e) => patch({ host: e.target.value })}
                  />
                </label>
                <label>
                  Port
                  <input
                    type="number"
                    required
                    min="1"
                    max="65535"
                    value={c.port}
                    onChange={(e) => patch({ port: Number(e.target.value) })}
                  />
                </label>
              </div>
              <div className="form-grid">
                <label>
                  Username
                  <input
                    autoComplete="off"
                    value={c.username}
                    placeholder={c.kind === "redis" ? "default" : "Username"}
                    onChange={(e) => patch({ username: e.target.value })}
                  />
                </label>
                <label>
                  Password
                  <input
                    type="password"
                    autoComplete="new-password"
                    value={c.password || ""}
                    placeholder={
                      c.hasPassword
                        ? "Saved in keychain · leave blank to keep"
                        : "Password"
                    }
                    onChange={(e) => patch({ password: e.target.value })}
                  />
                </label>
              </div>
              {c.kind !== "kafka" && (
                <label>
                  Database
                  <input
                    required={c.kind === "mongo"}
                    placeholder={c.kind === "redis" ? "0" : "Database name"}
                    value={c.database}
                    onChange={(e) => patch({ database: e.target.value })}
                  />
                </label>
              )}
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={c.tls}
                  onChange={(e) => patch({ tls: e.target.checked })}
                />
                <LockKeyhole size={15} /> Use TLS with certificate verification
              </label>
              {c.kind === "clickhouse" && (
                <p className="field-help">
                  Use the HTTP interface: port 8123, or 8443 with TLS for
                  ClickHouse Cloud.
                </p>
              )}
              {c.kind === "mongo" && (
                <label>
                  Authentication database
                  <input
                    value={c.authSource || ""}
                    placeholder="admin"
                    onChange={(e) => patch({ authSource: e.target.value })}
                  />
                  <span className="field-help">
                    The database where this user was created. Defaults to admin.
                  </span>
                </label>
              )}
            </>
          )}
          <SSHFields
            value={c.ssh}
            onChange={(ssh) => patch({ ssh })}
            onError={setError}
            cluster={cluster}
          />
          {!desktop && (
            <div className="notice">
              Browser preview. Open the desktop app to save and test live
              connections.
            </div>
          )}
          {error && (
            <div role="alert" className="error-box">
              {error}
            </div>
          )}
          {success && (
            <div role="status" className="success-box">
              <Check size={15} />
              {success}
            </div>
          )}
        </div>
        <div className="modal-footer">
          <span className="field-help">
            <LockKeyhole size={13} /> Secrets stay in your OS keychain
          </span>
          <button
            type="button"
            className="button"
            disabled={!!busy || !desktop}
            onClick={() => void action(true)}
          >
            {busy === "test" ? (
              <LoaderCircle className="spin" size={15} />
            ) : (
              <PlugZap size={15} />
            )}{" "}
            Test connection
          </button>
          <button className="button primary" disabled={!!busy || !desktop}>
            {busy === "save" && <LoaderCircle className="spin" size={15} />}Save
            connection
          </button>
        </div>
      </form>
    </Modal>
  );
}
