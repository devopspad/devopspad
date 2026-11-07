import { useEffect, useState } from "react";
import { LoaderCircle, RefreshCw } from "lucide-react";
import { api, errorText } from "./api";
import type { Connection, Workload } from "./types";
import type { OpsApplication, OpsEnvironment } from "./ops-api";
import { OpsConfigMapEditor, type ConfigMap } from "./OpsConfigMapEditor";
import { OpsNginxConfig } from "./OpsNginxConfig";
type KeyRef = {
  name: string;
  key?: string;
};
type Container = {
  name: string;
  env?: {
    name: string;
    value?: string;
    valueFrom?: {
      configMapKeyRef?: KeyRef;
      secretKeyRef?: KeyRef;
      fieldRef?: {
        fieldPath: string;
      };
      resourceFieldRef?: {
        resource: string;
      };
    };
  }[];
  envFrom?: {
    prefix?: string;
    configMapRef?: KeyRef;
    secretRef?: KeyRef;
  }[];
  volumeMounts?: {
    name: string;
    mountPath: string;
    subPath?: string;
  }[];
};
type PodSpec = {
  containers?: Container[];
  initContainers?: Container[];
  volumes?: {
    name: string;
    configMap?: KeyRef;
    secret?: {
      secretName: string;
    };
    projected?: {
      sources?: {
        configMap?: KeyRef;
        secret?: KeyRef;
      }[];
    };
  }[];
};
type ConfigRow = {
  container: string;
  name: string;
  value: string;
};
function configuration(spec: PodSpec) {
  const names = new Set<string>();
  const rows: ConfigRow[] = [];
  const source = (kind: string, ref: KeyRef) =>
    `${kind} / ${ref.name}${ref.key ? ` / ${ref.key}` : ""}`;
  for (const container of [
    ...(spec.initContainers || []),
    ...(spec.containers || []),
  ]) {
    const add = (name: string, value: string) =>
      rows.push({ container: container.name, name, value });
    for (const env of container.env || []) {
      const from = env.valueFrom;
      if (from?.configMapKeyRef) {
        names.add(from.configMapKeyRef.name);
        add(env.name, source("ConfigMap", from.configMapKeyRef));
      } else if (from?.secretKeyRef)
        add(env.name, source("Secret (reference only)", from.secretKeyRef));
      else
        add(
          env.name,
          env.value ??
            from?.fieldRef?.fieldPath ??
            from?.resourceFieldRef?.resource ??
            "—",
        );
    }
    for (const from of container.envFrom || []) {
      if (from.configMapRef) {
        names.add(from.configMapRef.name);
        add(`${from.prefix || ""}*`, source("ConfigMap", from.configMapRef));
      } else if (from.secretRef)
        add(
          `${from.prefix || ""}*`,
          source("Secret (reference only)", from.secretRef),
        );
    }
    for (const mount of container.volumeMounts || []) {
      const volume = spec.volumes?.find((v) => v.name === mount.name);
      if (!volume) continue;
      const path =
        mount.mountPath + (mount.subPath ? ` (${mount.subPath})` : "");
      if (volume.configMap) {
        names.add(volume.configMap.name);
        add(path, source("ConfigMap", volume.configMap));
      }
      if (volume.secret)
        add(
          path,
          source("Secret (reference only)", { name: volume.secret.secretName }),
        );
      for (const item of volume.projected?.sources || []) {
        if (item.configMap) {
          names.add(item.configMap.name);
          add(path, source("ConfigMap", item.configMap));
        }
        if (item.secret) add(path, source("Secret (reference only)", item.secret));
      }
    }
  }
  return { names: [...names].sort(), rows };
}
// Bound concurrent inspections for applications with many workload references.
async function mapLimit<T, R>(
  items: T[],
  read: (item: T) => Promise<R>,
): Promise<R[]> {
  const results: R[] = new Array(items.length);
  let cursor = 0;
  await Promise.all(
    Array.from({ length: Math.min(4, items.length) }, async () => {
      while (cursor < items.length) {
        const index = cursor++;
        results[index] = await read(items[index]);
      }
    }),
  );
  return results;
}
export function OpsApplicationConfig({
  projectRoot,
  app,
  env,
  connection,
  connectionError,
  saving,
  onBusy,
}: {
  projectRoot: string;
  app: OpsApplication;
  env: OpsEnvironment;
  connection?: Connection;
  connectionError: string;
  saving: boolean;
  onBusy: (busy: boolean) => void;
}) {
  const [version, setVersion] = useState(0);
  const [loading, setLoading] = useState(true);
  const [errors, setErrors] = useState<string[]>([]);
  const [workloads, setWorkloads] = useState<
    {
      workload: Workload;
      rows: ConfigRow[];
    }[]
  >([]);
  const [maps, setMaps] = useState<ConfigMap[]>([]);
  useEffect(() => {
    let stale = false;
    setLoading(true);
    setErrors([]);
    setWorkloads([]);
    setMaps([]);
    async function read() {
      if (!connection) throw new Error(connectionError);
      const cluster = await api.ReadOpsWorkloads(
        connection.id,
        env.namespace,
        app.selector,
      );
      if (stale) return;
      const warnings = [...cluster.warnings];
      if (cluster.truncated)
        warnings.push("The workload list is incomplete. Narrow the application selector.");
      const names = new Set<string>();
      const found = await mapLimit(cluster.workloads, async (workload) => {
        if (stale) return { workload, rows: [] };
        try {
          const detail = await api.InspectClusterResource(connection.id, {
            context: connection.context,
            namespace: env.namespace,
            resource: workload.resource,
            name: workload.name,
            uid: workload.uid,
          });
          const object = JSON.parse(detail.definition) as {
            spec?: {
              template?: {
                spec?: PodSpec;
              };
            };
          };
          const config = configuration(object.spec?.template?.spec || {});
          config.names.forEach((name) => names.add(name));
          return { workload, rows: config.rows };
        } catch (e) {
          warnings.push(`${workload.kind} / ${workload.name}: ${errorText(e)}`);
          return { workload, rows: [] };
        }
      });
      const configMaps = await mapLimit(
        [...names].sort(),
        async (name): Promise<ConfigMap> => {
          if (stale) return { name };
          try {
            const detail = await api.InspectClusterResource(connection.id, {
              context: connection.context,
              namespace: env.namespace,
              resource: "configmaps",
              name,
              uid: "",
            });
            const object = JSON.parse(detail.definition) as ConfigMap;
            return { ...object, name };
          } catch (e) {
            return { name, error: errorText(e) };
          }
        },
      );
      return { errors: warnings, workloads: found, maps: configMaps };
    }
    void read()
      .then((result) => {
        if (stale || !result) return;
        setErrors(result.errors);
        setWorkloads(result.workloads);
        setMaps(result.maps);
      })
      .catch((e) => {
        if (!stale) setErrors([errorText(e)]);
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [
    app.name,
    app.selector,
    env.name,
    env.namespace,
    connection,
    connectionError,
    version,
  ]);
  return (
    <section className="ops-panel">
      <div className="ops-panel-head">
        <div>
          <h1>App config</h1>
          <p>
            {app.name} · {env.name} / {env.namespace} · {"live cluster config"}
          </p>
        </div>
        <button
          className="ops-btn"
          aria-label="Refresh"
          disabled={loading || saving}
          onClick={() => setVersion((v) => v + 1)}
        >
          <RefreshCw size={15} />
          Refresh
        </button>
      </div>
      {loading ? (
        <div className="ops-loading" role="status">
          <LoaderCircle size={18} className="spin" />
          Loading app config…
        </div>
      ) : (
        <div className="ops-application-config">
          {errors.map((error, index) => (
            <div className="ops-error" role="alert" key={index}>
              {error}
            </div>
          ))}
          {!workloads.length && !errors.length && (
            <div className="ops-empty">
              <strong>No workloads found for this application</strong>
              <p>Check the namespace and selector in Project mapping: {app.selector}</p>
            </div>
          )}
          <h2>ConfigMap</h2>
          {maps.length ? (
            maps.map((map) => (
              <OpsConfigMapEditor
                key={`${connection?.id}:${connection?.context}:${env.namespace}:${map.name}`}
                map={map}
                connection={connection}
                namespace={env.namespace}
                saving={saving}
                onBusy={onBusy}
                onSaved={(updated) =>
                  setMaps((current) =>
                    current.map((item) =>
                      item.name === updated.name ? updated : item,
                    ),
                  )
                }
              />
            ))
          ) : (
            <p className="ops-muted">
              {errors.length
                ? "No ConfigMaps loaded yet. Check the error above."
                : "This application’s workloads reference no ConfigMaps."}
            </p>
          )}
          <OpsNginxConfig key={`${env.name}:${app.name}:${app.nginxPath || ""}`} projectRoot={projectRoot} environment={env.name} application={app.name} path={app.nginxPath} saving={saving} onBusy={onBusy} />
          <h2>Environment variables and mounts</h2>
          {workloads.map(({ workload, rows }) => (
            <section
              className="ops-config-card"
              key={`${workload.kind}/${workload.name}`}
            >
              <h3>
                {workload.kind} / {workload.name}
              </h3>
              {rows.length ? (
                <table className="ops-table ops-config-table">
                  <thead>
                    <tr>
                      <th>Container</th>
                      <th>Variable / mount path</th>
                      <th>Value / source</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((row, index) => (
                      <tr key={index}>
                        <td>{row.container}</td>
                        <td>{row.name}</td>
                        <td>
                          <pre>{row.value}</pre>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p className="ops-muted">No environment variables or config mounts found.</p>
              )}
            </section>
          ))}
        </div>
      )}
    </section>
  );
}
