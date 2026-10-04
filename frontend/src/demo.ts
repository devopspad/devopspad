import type {
  Connection,
  Kind,
  QueryResult,
  Resource,
  ResourceDetails,
} from "./types";
import { engines } from "./types";
import initSqlJs from "sql.js";
import wasmURL from "sql.js/dist/sql-wasm.wasm?url";

export const demoConnections: Connection[] = (
  Object.keys(engines) as Kind[]
).map((kind, i) => ({
  id: `demo-${kind}`,
  kind,
  name: [
    "Commerce database",
    "Session cache",
    "Product catalog",
    "Event pipeline",
    "Product analytics",
    "US East cluster",
    "Edge cluster",
  ][i],
  host: [
    "mysql.demo.local",
    "redis.demo.local",
    "mongo.demo.local",
    "kafka.demo.local",
    "analytics.demo.local",
    "us-east.demo.local",
    "edge.demo.local",
  ][i],
  port: engines[kind].port,
  database:
    kind === "redis"
      ? "0"
      : kind === "k8s" || kind === "k3s"
        ? "default"
        : "commerce",
  username: "readonly",
  environment: i === 6 ? "Development" : i === 5 ? "Staging" : "Production",
  tls: true,
  kubeconfig: "",
  context: "",
  hasPassword: false,
}));
export const initialQuery =
  "SELECT\n  id, customer, email, total, status, created_at\nFROM orders\nORDER BY created_at DESC\nLIMIT 100;";
const people = [
  ["Olivia Rhye", "olivia@acme.com", 249, "Completed"],
  ["Phoenix Baker", "phoenix@layers.com", 129, "Completed"],
  ["Lana Steiner", "lana@sisyphus.com", 549, "Processing"],
  ["Demi Wilkinson", "demi@catalog.com", 89, "Completed"],
  ["Drew Cano", "drew@circooles.com", 199, "Completed"],
  ["Natali Craig", "natali@hourglass.com", 349, "Pending"],
  ["Orlando Diggs", "orlando@command.com", 79, "Completed"],
  ["Andi Lane", "andi@quotient.com", 429, "Processing"],
  ["Kate Morrison", "kate@feather.com", 159, "Completed"],
  ["Koray Okumus", "koray@bolt.com", 299, "Completed"],
  ["Alex Morgan", "alex@linear.app", 189, "Pending"],
  ["Sam Rivera", "sam@notion.so", 649, "Completed"],
];
const sqlEngine = initSqlJs({ locateFile: () => wasmURL });
const database = sqlEngine.then((SQL) => {
  const db = new SQL.Database();
  db.run(
    "CREATE TABLE orders (id INTEGER PRIMARY KEY, customer TEXT, email TEXT, total REAL, status TEXT, created_at TEXT); CREATE TABLE customers (id INTEGER, name TEXT, email TEXT); CREATE TABLE products (id INTEGER, name TEXT, price REAL); CREATE TABLE payments (id INTEGER, order_id INTEGER, amount REAL); CREATE TABLE subscriptions (id INTEGER, customer_id INTEGER, plan TEXT);",
  );
  people.forEach((p, i) => {
    db.run("INSERT INTO orders VALUES (?, ?, ?, ?, ?, ?)", [
      1048 - i,
      ...p,
      `2026-10-01 ${String(14 - Math.floor(i / 4)).padStart(2, "0")}:${String(52 - i * 3).padStart(2, "0")}:00`,
    ]);
    db.run("INSERT INTO customers VALUES (?, ?, ?)", [i + 1, p[0], p[1]]);
  });
  db.run(
    "INSERT INTO products VALUES (1, 'Pro workspace', 49), (2, 'Team workspace', 129), (3, 'Enterprise workspace', 499); INSERT INTO payments VALUES (1, 1048, 249); INSERT INTO subscriptions VALUES (1, 1, 'Pro')",
  );
  db.run("CREATE INDEX idx_orders_status ON orders(status)");
  db.run("PRAGMA query_only=ON");
  return db;
});
const sandboxDatabase = sqlEngine.then((SQL) => {
  const db = new SQL.Database();
  db.run(
    "CREATE TABLE experiments (id INTEGER PRIMARY KEY, name TEXT, status TEXT); INSERT INTO experiments VALUES (1, 'Checkout redesign', 'Running'); PRAGMA query_only=ON",
  );
  return db;
});
export function demoDatabases(kind: Kind): string[] {
  if (kind === "k8s" || kind === "k3s") return ["*", "default", "kube-system"];
  return kind === "mongo" ? ["commerce"] : ["commerce", "sandbox"];
}
export function demoResources(kind: Kind, scope = "commerce"): Resource[] {
  if ((kind === "mysql" || kind === "clickhouse") && scope === "sandbox")
    return [
      {
        name: "experiments",
        kind: "table",
        query: "SELECT * FROM experiments LIMIT 100;",
      },
    ];
  if (kind === "mysql" || kind === "clickhouse")
    return ["customers", "orders", "payments", "products", "subscriptions"].map(
      (name) => ({
        name,
        kind: "table",
        query:
          name === "orders" ? initialQuery : `SELECT * FROM ${name} LIMIT 100;`,
      }),
    );
  if (kind === "redis")
    return ["session:user:1048", "cache:featured", "rate-limit:api"].map(
      (name) => ({ name, kind: "key", query: `GET ${name}` }),
    );
  if (kind === "mongo")
    return ["products", "reviews"].map((name) => ({
      name,
      kind: "collection",
      query: JSON.stringify(
        { collection: name, filter: {}, limit: 100 },
        null,
        2,
      ),
    }));
  if (kind === "kafka")
    return ["order.created", "payment.processed", "user.activity"].map(
      (name) => ({ name, kind: "topic", query: name }),
    );
  return ["pods", "deployments", "services", "nodes", "namespaces"].map(
    (name) => ({ name, kind: "resource", query: `get ${name}` }),
  );
}
export async function demoExecute(
  kind: Kind,
  query: string,
  scope = "commerce",
): Promise<QueryResult> {
  const start = performance.now();
  let columns: string[] = [];
  let rows: unknown[][] = [];
  if (kind === "mysql" || kind === "clickhouse") {
    const db = await (scope === "sandbox" ? sandboxDatabase : database);
    if (/^\s*show\s+tables\s*;?\s*$/i.test(query)) {
      columns = ["table"];
      rows = demoResources(kind, scope).map((r) => [r.name]);
    } else {
      if (!/^\s*(select|with|explain)\b/i.test(query))
        throw new Error(
          "The demo supports read-only SQLite SELECT queries. Connect a server for its native SQL dialect.",
        );
      const stmt = db.prepare(query);
      try {
        columns = stmt.getColumnNames();
        while (stmt.step() && rows.length < 500) rows.push(stmt.get());
      } finally {
        stmt.free();
      }
    }
  } else if (kind === "redis") {
    const [cmd, key] = query.trim().split(/\s+/);
    columns = ["key", "value", "type"];
    const data: Record<string, string> = {
      "session:user:1048": '{"userId":1048,"plan":"pro"}',
      "cache:featured": '["workspace","analytics"]',
      "rate-limit:api": "42",
    };
    if (cmd.toUpperCase() === "SCAN")
      rows = Object.entries(data).map(([k, v]) => [k, v, "string"]);
    else if (cmd.toUpperCase() === "GET")
      rows = [[key, data[key] ?? null, "string"]];
    else
      throw new Error(
        "The Redis demo supports SCAN and GET. Connect a server for more commands.",
      );
  } else if (kind === "mongo") {
    const input = JSON.parse(query);
    if (!["products", "reviews"].includes(input.collection))
      throw new Error("Demo collections: products, reviews");
    const docs =
      input.collection === "products"
        ? [
            { _id: "prd_01", name: "Pro workspace", price: 49, active: true },
            { _id: "prd_02", name: "Team workspace", price: 129, active: true },
          ]
        : [{ _id: "rev_01", rating: 5, text: "A calmer way to work." }];
    columns = ["document"];
    const limit = input.limit ?? 100;
    const skip = input.skip ?? 0;
    if (
      !Number.isInteger(limit) ||
      limit < 1 ||
      limit > 500 ||
      !Number.isInteger(skip) ||
      skip < 0 ||
      skip > 1000000
    )
      throw new Error("Limit must be 1–500 and skip must be 0–1000000.");
    const sort: Record<string, number> = input.sort || { _id: 1 };
    if (!Object.values(sort).every((v) => v === 1 || v === -1))
      throw new Error("Sort directions must be 1 or -1.");
    if (!("_id" in sort)) sort._id = 1;
    const matches = docs
      .filter((doc: Record<string, unknown>) =>
        Object.entries(input.filter || {}).every(([k, v]) => {
          if (v && typeof v === "object")
            throw new Error(
              "The demo supports equality filters. Connect MongoDB for Extended JSON and operators.",
            );
          return doc[k] === v;
        }),
      )
      .sort((a, b) => {
        for (const [key, direction] of Object.entries(sort)) {
          const av = (a as Record<string, any>)[key],
            bv = (b as Record<string, any>)[key];
          if (av < bv) return -direction;
          if (av > bv) return direction;
        }
        return 0;
      });
    const projection = input.projection || {};
    if (!Object.values(projection).every((v) => v === 0 || v === 1))
      throw new Error("The demo supports 0/1 projection fields.");
    const included = Object.entries(projection)
      .filter(([k, v]) => k !== "_id" && v === 1)
      .map(([k]) => k);
    if (
      included.length &&
      Object.entries(projection).some(([k, v]) => k !== "_id" && v === 0)
    )
      throw new Error("Cannot mix inclusion and exclusion projections.");
    rows = matches.slice(skip, skip + limit).map((doc) => [
      Object.fromEntries(
        Object.entries(doc).filter(([key]) => {
          if (key === "_id") return projection._id !== 0;
          return included.length || projection._id === 1
            ? included.includes(key)
            : projection[key] !== 0;
        }),
      ),
    ]);
    const pageQuery = (offset: number) =>
      JSON.stringify({ ...input, sort, skip: offset, limit }, null, 2);
    return {
      columns,
      rows,
      duration: Math.max(1, Math.round(performance.now() - start)),
      truncated: skip + limit < matches.length,
      nextQuery:
        skip + limit < matches.length ? pageQuery(skip + limit) : undefined,
      previousQuery:
        skip > 0 ? pageQuery(Math.max(0, skip - limit)) : undefined,
      offset: skip,
      message: "Sample data · local demo",
    };
  } else if (kind === "kafka") {
    columns = ["topic", "partition", "leader", "replicas", "in_sync_replicas"];
    rows = ["order.created", "payment.processed", "user.activity"]
      .filter((t) => query.trim() === "topics" || t === query.trim())
      .flatMap((t) => [0, 1, 2].map((p) => [t, p, p + 1, 3, 3]));
  } else {
    const name = query.trim().split(/\s+/)[1];
    if (
      !query.startsWith("get ") ||
      !demoResources(kind).some((r) => r.name === name)
    )
      throw new Error(
        "Try: get pods, get deployments, get services, get nodes, or get namespaces",
      );
    columns = ["name", "namespace", "status", "ready", "age"];
    rows =
      name === "pods"
        ? [
            ["api-7d9f8b6c4-x2k9p", "default", "Running", "1/1", "3d 4h"],
            ["worker-65b8d9c7f-q8m2r", "default", "Running", "1/1", "3d 4h"],
            [
              "scheduler-84c7f9d6b-p3n5k",
              "default",
              "Running",
              "1/1",
              "2d 18h",
            ],
          ]
        : [[`${name}-demo`, "default", "Active", "1/1", "14d"]];
    const namespace = query.trim().split(/\s+/)[2] || scope;
    if (name === "namespaces")
      rows = ["default", "kube-system"].map((n) => [
        n,
        "",
        "Active",
        "—",
        "14d",
      ]);
    else if (name === "nodes")
      rows = rows.map((r) => [r[0], "", ...r.slice(2)]);
    else if (namespace && namespace !== "*")
      rows = rows.filter((r) => r[1] === namespace);
  }
  return {
    columns,
    rows,
    resourceType:
      kind === "k8s" || kind === "k3s"
        ? query.trim().split(/\s+/)[1]
        : undefined,
    duration: Math.max(1, Math.round(performance.now() - start)),
    truncated: rows.length >= 500,
    message: "Sample data · local demo",
  };
}

export async function demoInspect(
  kind: Kind,
  scope: string,
  name: string,
): Promise<ResourceDetails> {
  const empty = (): QueryResult => ({
    columns: [],
    rows: [],
    duration: 0,
    truncated: false,
    message: "",
  });
  const details: ResourceDetails = {
    name,
    kind,
    database: scope,
    properties: [{ name: "Database", value: scope }],
    columns: empty(),
    indexes: empty(),
    preview: empty(),
    definition: "",
  };
  if (kind === "redis") {
    const result = await demoExecute(kind, `GET ${name}`);
    details.properties.push(
      { name: "Type", value: "string" },
      { name: "TTL", value: "No expiration (sample)" },
    );
    details.preview = {
      ...result,
      columns: ["value"],
      rows: [[result.rows[0]?.[1]]],
    };
    return details;
  }
  const db = await (scope === "sandbox" ? sandboxDatabase : database);
  const quoted = `'${name.replaceAll("'", "''")}'`;
  const columns = db.exec(`PRAGMA table_info(${quoted})`)[0];
  if (!columns) throw new Error("Table was not found in the demo database.");
  details.columns = {
    ...empty(),
    columns: ["Name", "Type", "Nullable", "Default", "Primary key"],
    rows: columns.values.map((r) => [
      r[1],
      r[2],
      r[3] || r[5] ? "No" : "Yes",
      r[4],
      r[5] ? "Yes" : "No",
    ]),
  };
  const indexes = db.exec(`PRAGMA index_list(${quoted})`)[0];
  if (indexes)
    details.indexes = {
      ...empty(),
      columns: ["Name", "Unique", "Origin"],
      rows: indexes.values.map((r) => [r[1], r[2] ? "Yes" : "No", r[3]]),
    };
  const ddl = db.exec(
    `SELECT sql FROM sqlite_master WHERE type='table' AND name=${quoted}`,
  )[0];
  details.definition = String(ddl?.values[0]?.[0] || "");
  return details;
}
