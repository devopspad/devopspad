import { useState } from "react";
import { Check, CircleAlert, Clock3, Terminal, Rocket } from "lucide-react";
import { Modal } from "./Modal";
import type { OpsArtifact, OpsRun } from "./ops-api";

const at = (s?: string) =>
  s ? new Date(s).toLocaleString("zh-CN", { hour12: false }) : "—";
const status = (s: string) =>
  s === "succeeded"
    ? "上报成功"
    : s === "failed"
      ? "执行失败"
      : s === "cancelled"
        ? "已取消"
        : "执行中 / 待核实";
export function OpsRuns({
  runs,
  artifacts,
  onRelease,
  selectedId,
  onSelected,
}: {
  runs: OpsRun[];
  artifacts: OpsArtifact[];
  onRelease: (a: OpsArtifact) => void;
  selectedId?: string;
  onSelected: (id?: string) => void;
}) {
  const [kind, setKind] = useState("");
  const [query, setQuery] = useState("");
  const selected = runs.find((r) => r.id === selectedId);
  const visible = runs.filter(
    (r) =>
      (!kind || r.kind === kind) &&
      `${r.version} ${r.actor.name} ${r.source.commit} ${r.id}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  return (
    <div className="ops-table-pad">
      <div className="ops-run-filter">
        <select
          aria-label="执行类型"
          value={kind}
          onChange={(e) => setKind(e.target.value)}
        >
          <option value="">全部执行类型</option>
          <option value="build">AI 构建</option>
          <option value="release">外部发布</option>
        </select>
        <input
          aria-label="筛选执行记录"
          placeholder="版本、提交或执行者"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <span className="ops-muted">每 10 秒读取本地记录</span>
      </div>
      <div className="ops-table-wrap bordered">
        <table className="ops-table">
          <thead>
            <tr>
              <th>执行 / 版本</th>
              <th>源码提交</th>
              <th>执行者</th>
              <th>目标</th>
              <th>状态</th>
              <th>开始时间</th>
              <th className="ops-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((r) => (
              <tr key={r.id}>
                <td>
                  <button
                    className="ops-link neutral"
                    onClick={() => onSelected(r.id)}
                  >
                    <Terminal size={15} />
                    {r.kind === "build" ? "构建" : "发布"} · {r.version}
                  </button>
                  <small>{r.id.slice(0, 8)}</small>
                </td>
                <td>
                  <code>{r.source.commit.slice(0, 12)}</code>
                  <small>
                    {r.source.branch}
                    {r.source.dirty ? " · 有未提交改动" : ""}
                  </small>
                </td>
                <td>
                  {r.actor.name}
                  <small>
                    {r.actor.kind === "ai" ? "AI 执行" : "人工执行"}
                  </small>
                </td>
                <td>
                  {r.target
                    ? `${r.target.environment} / ${r.target.workload}`
                    : "制品库"}
                </td>
                <td>
                  <span className={`ops-status ${r.status}`}>
                    {status(r.status)}
                  </span>
                </td>
                <td className="ops-muted">{at(r.startedAt)}</td>
                <td className="ops-right">
                  <button className="ops-link" onClick={() => onSelected(r.id)}>
                    详情
                  </button>
                  {r.kind === "build" &&
                    r.status === "succeeded" &&
                    artifacts.some((a) => a.runId === r.id) && (
                      <button
                        className="ops-link"
                        onClick={() =>
                          onRelease(artifacts.find((a) => a.runId === r.id)!)
                        }
                      >
                        <Rocket size={14} />
                        发布制品
                      </button>
                    )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!visible.length && (
          <div className="ops-empty">
            <Terminal size={28} />
            <strong>暂无执行记录</strong>
            <p>
              让 AI 在执行前调用 devopspadctl runs record
              上报开始，再记录步骤和结果。
            </p>
          </div>
        )}
      </div>
      <p className="ops-footnote">
        构建成功后自动生成制品索引。这里显示执行端上报的事实；外部发布上报不会触发集群操作，也不等同于本应用独立验收。
      </p>
      {selected && (
        <Modal
          wide
          title={`${selected.kind === "build" ? "构建" : "发布"} ${selected.version}`}
          subtitle={`${selected.actor.name} · ${selected.id}`}
          onClose={() => onSelected(undefined)}
        >
          <div className="ops-dialog-body ops-form">
            <span className={`ops-status ${selected.status}`}>
              {status(selected.status)}
            </span>
            <div className="ops-release-images">
              <label>
                源码
                <code>
                  {selected.source.repository}
                  <br />
                  {selected.source.commit} · {selected.source.branch}
                  {selected.source.dirty ? " · 工作区有未提交改动" : ""}
                </code>
              </label>
              <label>
                AI / 执行会话
                <code>
                  {selected.actor.name} · {selected.actor.sessionId}
                </code>
              </label>
              <label>
                构建 / 发布方式<code>{selected.recipe}</code>
              </label>
              {selected.image && (
                <label>
                  镜像摘要<code>{selected.image}</code>
                </label>
              )}
              {selected.target && (
                <label>
                  发布目标
                  <code>
                    {selected.connection} / {selected.namespace} /{" "}
                    {selected.target.workload} / {selected.target.container}
                    <br />
                    构建 {selected.target.buildId}
                  </code>
                </label>
              )}
            </div>
            <ol className="ops-steps ops-run-steps">
              {selected.events.map((saved) => (
                <li
                  key={saved.event.id}
                  className={
                    saved.event.step?.status === "failed" ||
                    (saved.event.finish &&
                      saved.event.finish.status !== "succeeded")
                      ? "failed"
                      : ""
                  }
                >
                  {saved.event.step?.status === "failed" ||
                  (saved.event.finish &&
                    saved.event.finish.status !== "succeeded") ? (
                    <CircleAlert size={15} />
                  ) : saved.event.type === "start" ? (
                    <Clock3 size={15} />
                  ) : (
                    <Check size={15} />
                  )}
                  <div>
                    <strong>
                      {saved.event.type === "start"
                        ? "开始执行"
                        : saved.event.type === "finish"
                          ? status(selected.status)
                          : saved.event.step?.name}
                    </strong>
                    <small>{at(saved.at)}</small>
                    <p>
                      {saved.event.step?.message || saved.event.finish?.message}
                    </p>
                    {saved.event.step?.logPath && (
                      <code>日志：{saved.event.step.logPath}</code>
                    )}
                  </div>
                </li>
              ))}
            </ol>
            <p>
              {selected.message ||
                "尚未上报结束；若执行进程中断，请核实后补报失败或取消。"}
            </p>
            <p className="ops-muted">
              {at(selected.startedAt)} → {at(selected.finishedAt)}
            </p>
          </div>
        </Modal>
      )}
    </div>
  );
}
