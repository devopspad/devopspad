import { useEffect, useState } from "react";
import { api, errorText } from "./api";
import {
  ConfigCodeEditor,
  configLanguage,
  validateConfig,
} from "./ConfigCodeEditor";
import type { Connection } from "./types";

export type ConfigMap = {
  name: string;
  metadata?: { uid: string; resourceVersion: string };
  immutable?: boolean;
  data?: Record<string, string>;
  binaryData?: Record<string, string>;
  error?: string;
};
type Draft = {
  data: Record<string, string>;
  uid: string;
  resourceVersion: string;
};
// Keep unsaved edits across application/page switches, without persisting config
// contents to browser storage. Identity and version always travel with a draft.
const drafts = new Map<string, Draft>();

export function OpsConfigMapEditor({
  map,
  connection,
  namespace,
  saving,
  onBusy,
  onSaved,
}: {
  map: ConfigMap;
  connection?: Connection;
  namespace: string;
  saving: boolean;
  onBusy: (busy: boolean) => void;
  onSaved: (map: ConfigMap) => void;
}) {
  const identity = JSON.stringify([
    connection?.id,
    connection?.context,
    namespace,
    map.name,
  ]);
  const [draft, setDraft] = useState<Draft | undefined>(() =>
    drafts.get(identity),
  );
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const data = draft?.data || map.data || {};
  const changed =
    !!draft && JSON.stringify(draft.data) !== JSON.stringify(map.data || {});
  useEffect(() => {
    if (!draft) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [draft]);
  function edit(next: Draft | undefined) {
    if (next) drafts.set(identity, next);
    else drafts.delete(identity);
    setDraft(next);
    setError("");
    setNotice("");
  }
  async function save() {
    if (!draft || !connection || saving) return;
    setError("");
    for (const [filename, value] of Object.entries(draft.data)) {
      if (value === map.data?.[filename]) continue;
      try {
        validateConfig(filename, value);
      } catch (e) {
        setError(`${filename}: ${errorText(e)}`);
        return;
      }
    }
    onBusy(true);
    try {
      const definition = await api.SaveConfigMap(connection.id, {
        ref: {
          context: connection.context || "",
          namespace,
          resource: "configmaps",
          name: map.name,
          uid: draft.uid,
        },
        resourceVersion: draft.resourceVersion,
        data: draft.data,
      });
      const updated = JSON.parse(definition) as ConfigMap;
      onSaved({ ...updated, name: map.name });
      edit(undefined);
      setNotice(
        "Saved to the cluster. Environment variables and subPath mounts take effect after a restart; other mounts depend on how the app reloads.",
      );
    } catch (e) {
      setError(errorText(e));
    } finally {
      onBusy(false);
    }
  }
  return (
    <section className="ops-config-card" aria-label={`ConfigMap ${map.name}`}>
      <div className="ops-config-card-head">
        <h3>{map.name}</h3>
        {!map.error && (
          <div className="ops-config-actions">
            {draft ? (
              <>
                <span className="ops-muted">
                  {changed ? "Unsaved" : "Editing"}
                </span>
                <button
                  className="ops-btn"
                  disabled={saving}
                  onClick={() => edit(undefined)}
                >
                  Cancel editing
                </button>
                <button
                  className="ops-btn primary"
                  disabled={saving || !changed || !connection}
                  onClick={() => void save()}
                >
                  {saving ? "Saving…" : "Save to cluster"}
                </button>
              </>
            ) : (
              <button
                className="ops-btn"
                disabled={
                  saving ||
                  !connection ||
                  map.immutable ||
                  !map.metadata?.uid ||
                  !map.metadata.resourceVersion ||
                  !Object.keys(data).length
                }
                onClick={() =>
                  edit({
                    data: { ...data },
                    uid: map.metadata!.uid,
                    resourceVersion: map.metadata!.resourceVersion,
                  })
                }
              >
                Edit config
              </button>
            )}
          </div>
        )}
      </div>
      {map.immutable && (
        <p className="ops-muted">This ConfigMap is immutable.</p>
      )}
      {(map.error || error) && (
        <div className="ops-error" role="alert">
          {map.error || error}
        </div>
      )}
      {notice && <p role="status">{notice}</p>}
      {!map.error && (
        <>
          {Object.entries(data).map(([filename, value]) => (
            <div className="ops-config-value" key={filename}>
              <h4>
                {filename}{" "}
                <span className="ops-muted">{configLanguage(filename)}</span>
              </h4>
              <ConfigCodeEditor
                filename={filename}
                label={`${map.name} / ${filename}`}
                value={value}
                editable={!!draft && !saving}
                onChange={(value) => {
                  if (draft && draft.data[filename] !== value)
                    edit({
                      ...draft,
                      data: { ...draft.data, [filename]: value },
                    });
                }}
              />
            </div>
          ))}
          {Object.keys(map.binaryData || {}).map((key) => (
            <p className="ops-muted" key={key}>
              {key} · binary data (read-only)
            </p>
          ))}
          {!Object.keys(data).length &&
            !Object.keys(map.binaryData || {}).length && (
              <p className="ops-muted">This ConfigMap has no data.</p>
            )}
        </>
      )}
    </section>
  );
}
