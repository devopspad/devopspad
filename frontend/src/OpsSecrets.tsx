import { useState } from "react";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { api, errorText } from "./api";
import type { OpsBridge, OpsPlan, OpsRecord } from "./ops-api";

type SecretPlan = {
  id: string;
  namespace: string;
  name: string;
  type: string;
  create: boolean;
  keys: { key: string; change: string }[];
  contentSha256: string;
};
type SecretInfo = { name: string; type: string; keys: string[]; updatedAt?: string };
type SecretsBridge = OpsBridge & {
  DescribeOpsSecret(environment: string, application: string, name: string): Promise<SecretInfo>;
  PlanOpsSecret(
    environment: string,
    application: string,
    request: { name: string; type: string; data: Record<string, string>; server: string; username: string; password: string },
  ): Promise<SecretPlan>;
  ApplyOpsSecret(plan: string): Promise<OpsRecord>;
  PlanOpsPatch(
    environment: string,
    application: string,
    workload: string,
    patch: {
      container?: string;
      addImagePullSecrets?: string[];
      addEnvFromSecrets?: string[];
      setEnvSecretRefs?: { name: string; secret: string; key: string }[];
    },
  ): Promise<OpsPlan & { changes?: string[] }>;
};
const bridge = api as unknown as SecretsBridge;

type Row = { key: string; value: string };
type EnvRow = { name: string; secret: string; key: string };

/** Secret values typed here stay in the desktop process for one plan and are never shown again. */
export function OpsSecrets({ environment, application, busy, onBusy }: { environment: string; application: string; busy: boolean; onBusy: (b: boolean) => void }) {
  const [name, setName] = useState("");
  const [type, setType] = useState("generic");
  const [rows, setRows] = useState<Row[]>([{ key: "", value: "" }]);
  const [server, setServer] = useState("ghcr.io");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [info, setInfo] = useState<SecretInfo>();
  const [plan, setPlan] = useState<SecretPlan>();
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const [workload, setWorkload] = useState(application);
  const [pullSecret, setPullSecret] = useState("");
  const [container, setContainer] = useState("");
  const [envFrom, setEnvFrom] = useState("");
  const [envRows, setEnvRows] = useState<EnvRow[]>([]);
  const [patchPlan, setPatchPlan] = useState<OpsPlan & { changes?: string[] }>();
  const [patchMessage, setPatchMessage] = useState("");
  const [patchError, setPatchError] = useState("");

  async function run(fn: () => Promise<void>, onError: (e: string) => void) {
    onBusy(true);
    onError("");
    try {
      await fn();
    } catch (e) {
      onError(errorText(e));
    } finally {
      onBusy(false);
    }
  }
  const clearValues = () => {
    setRows([{ key: "", value: "" }]);
    setPassword("");
  };

  return (
    <div className="ops-secrets">
      <section className="ops-secrets-block" aria-label="Secret">
        <h2><KeyRound size={15} /> Create or update a Secret</h2>
        <p className="ops-secrets-hint">
          The whole Secret is replaced: keys you do not list are removed. Values are sent to the cluster once and never shown or stored by DevOpsPad; records keep key names and a hash only.
        </p>
        <div className="ops-secrets-grid">
          <label>Name<input aria-label="Secret name" value={name} onChange={(e) => { setName(e.target.value); setPlan(undefined); setInfo(undefined); }} /></label>
          <label>Type
            <select aria-label="Secret type" value={type} onChange={(e) => { setType(e.target.value); setPlan(undefined); }}>
              <option value="generic">generic</option>
              <option value="docker-registry">docker-registry (image pull)</option>
            </select>
          </label>
          <button className="ops-btn" disabled={busy || !name} onClick={() => run(async () => setInfo(await bridge.DescribeOpsSecret(environment, application, name)), setError)}>
            Show current keys
          </button>
        </div>
        {info && (
          <p className="ops-secrets-hint" role="status">
            {info.name} ({info.type}): {info.keys.join(", ") || "no keys"}{info.updatedAt ? ` · last changed ${info.updatedAt}` : ""}
          </p>
        )}
        {type === "generic" ? (
          <div className="ops-secrets-rows">
            {rows.map((r, i) => (
              <div key={i} className="ops-secrets-row">
                <input aria-label={`Key ${i + 1}`} placeholder="KEY" value={r.key} onChange={(e) => setRows(rows.map((x, j) => (j === i ? { ...x, key: e.target.value } : x)))} />
                <textarea aria-label={`Value ${i + 1}`} placeholder="value" spellCheck={false} autoComplete="off" rows={1} value={r.value} onChange={(e) => setRows(rows.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)))} />
                <button className="ops-btn" aria-label={`Remove key ${i + 1}`} onClick={() => setRows(rows.filter((_, j) => j !== i))} disabled={rows.length === 1}>
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
            <button className="ops-btn" onClick={() => setRows([...rows, { key: "", value: "" }])}><Plus size={14} /> Add key</button>
          </div>
        ) : (
          <div className="ops-secrets-grid">
            <label>Registry<input aria-label="Registry server" value={server} onChange={(e) => setServer(e.target.value)} /></label>
            <label>Username<input aria-label="Registry username" value={username} onChange={(e) => setUsername(e.target.value)} /></label>
            <label>Password or token<input aria-label="Registry password" type="password" autoComplete="off" value={password} onChange={(e) => setPassword(e.target.value)} /></label>
          </div>
        )}
        <div className="ops-secrets-actions">
          <button className="ops-btn"
            disabled={busy || !name}
            onClick={() =>
              run(async () => {
                setMessage("");
                const data: Record<string, string> = {};
                for (const r of rows) if (r.key) data[r.key] = r.value;
                setPlan(await bridge.PlanOpsSecret(environment, application, { name, type, data, server, username, password }));
              }, setError)
            }
          >
            Plan
          </button>
          {plan && (
            <button
              className="ops-btn primary"
              disabled={busy}
              onClick={() =>
                run(async () => {
                  const r = await bridge.ApplyOpsSecret(plan.id);
                  setPlan(undefined);
                  clearValues();
                  setMessage(r.status === "succeeded" ? `Secret ${plan.create ? "created" : "updated"}.` : r.message);
                }, setError)
              }
            >
              Apply to {plan.namespace}
            </button>
          )}
        </div>
        {plan && (
          <ul className="ops-secrets-plan" aria-label="Secret plan">
            <li>{plan.create ? "Create" : "Update"} {plan.namespace}/{plan.name} ({plan.type}) · content sha256 {plan.contentSha256}</li>
            {plan.keys.map((k) => (
              <li key={k.key}><code>{k.key}</code> <span className={`ops-secrets-change ${k.change}`}>{k.change}</span></li>
            ))}
          </ul>
        )}
        {message && <p className="ops-secrets-ok" role="status">{message}</p>}
        {error && <p className="ops-secrets-error" role="alert">{error}</p>}
      </section>

      <section className="ops-secrets-block" aria-label="Use Secrets">
        <h2>Use Secrets in the Deployment</h2>
        <p className="ops-secrets-hint">Adds pull secrets and Secret-backed environment, then rolls the pods with the same lock and checks as a release. Literal environment values are never changed.</p>
        <div className="ops-secrets-grid">
          <label>Deployment<input aria-label="Deployment" value={workload} onChange={(e) => setWorkload(e.target.value)} /></label>
          <label>Add imagePullSecret<input aria-label="Add imagePullSecret" placeholder="ghcr-pull" value={pullSecret} onChange={(e) => setPullSecret(e.target.value)} /></label>
          <label>Container<input aria-label="Container" placeholder="app" value={container} onChange={(e) => setContainer(e.target.value)} /></label>
          <label>Add envFrom Secret<input aria-label="Add envFrom Secret" placeholder="app-keys" value={envFrom} onChange={(e) => setEnvFrom(e.target.value)} /></label>
        </div>
        <div className="ops-secrets-rows">
          {envRows.map((r, i) => (
            <div key={i} className="ops-secrets-row">
              <input aria-label={`Env name ${i + 1}`} placeholder="ENV_NAME" value={r.name} onChange={(e) => setEnvRows(envRows.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />
              <input aria-label={`Env secret ${i + 1}`} placeholder="secret" value={r.secret} onChange={(e) => setEnvRows(envRows.map((x, j) => (j === i ? { ...x, secret: e.target.value } : x)))} />
              <input aria-label={`Env key ${i + 1}`} placeholder="key" value={r.key} onChange={(e) => setEnvRows(envRows.map((x, j) => (j === i ? { ...x, key: e.target.value } : x)))} />
              <button className="ops-btn" aria-label={`Remove env ${i + 1}`} onClick={() => setEnvRows(envRows.filter((_, j) => j !== i))}><Trash2 size={14} /></button>
            </div>
          ))}
          <button className="ops-btn" onClick={() => setEnvRows([...envRows, { name: "", secret: "", key: "" }])}><Plus size={14} /> Add env from Secret key</button>
        </div>
        <div className="ops-secrets-actions">
          <button className="ops-btn"
            disabled={busy || !workload}
            onClick={() =>
              run(async () => {
                setPatchMessage("");
                setPatchPlan(
                  await bridge.PlanOpsPatch(environment, application, workload, {
                    container: container || undefined,
                    addImagePullSecrets: pullSecret ? [pullSecret] : [],
                    addEnvFromSecrets: envFrom ? [envFrom] : [],
                    setEnvSecretRefs: envRows.filter((r) => r.name),
                  }),
                );
              }, setPatchError)
            }
          >
            Plan
          </button>
          {patchPlan && (
            <button
              className="ops-btn primary"
              disabled={busy}
              onClick={() =>
                run(async () => {
                  const r = await bridge.ApplyOpsRelease(patchPlan.id);
                  setPatchPlan(undefined);
                  setPatchMessage(r.status === "succeeded" ? "Deployment updated and ready." : r.message);
                }, setPatchError)
              }
            >
              Apply and roll the pods
            </button>
          )}
        </div>
        {patchPlan && (
          <ul className="ops-secrets-plan" aria-label="Deployment plan">
            {(patchPlan.changes || []).map((c) => <li key={c}>{c}</li>)}
          </ul>
        )}
        {patchMessage && <p className="ops-secrets-ok" role="status">{patchMessage}</p>}
        {patchError && <p className="ops-secrets-error" role="alert">{patchError}</p>}
      </section>
    </div>
  );
}
