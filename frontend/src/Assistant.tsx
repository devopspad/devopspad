import { useState } from "react";
import {
  ArrowUp,
  ArrowUpRight,
  Check,
  Code2,
  Database,
  LoaderCircle,
  Settings2,
  ShieldCheck,
  Sparkles,
  Layers3,
  FileCode2,
  FolderOpen,
  X,
} from "lucide-react";
import { api, errorText } from "./api";
import { engines, type Kind } from "./types";
type Message = {
  role: "user" | "assistant";
  content: string;
};
export function Assistant({
  kind,
  names,
  onInsert,
  onClose,
  onSettings,
  connectionName,
  activeTitle,
  onFocusQuery,
}: {
  connectionName: string;
  activeTitle?: string;
  onFocusQuery: () => void;
  kind: Kind;
  names: string[];
  onInsert: (q: string) => void;
  onClose: () => void;
  onSettings: () => void;
}) {
  const [prompt, setPrompt] = useState("");
  const [context, setContext] = useState(false);
  const [messages, setMessages] = useState<Message[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function ask(text: string) {
    if (!text.trim() || busy) return;
    setPrompt("");
    setError("");
    setMessages((m) => [...m, { role: "user", content: text }]);
    setBusy(true);
    try {
      const content = await api.AskAI({
        prompt: text,
        kind,
        schema: context ? names : [],
      });
      setMessages((m) => [...m, { role: "assistant", content }]);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <aside className="assistant">
      <div className="panel-header">
        <span>
          DevOpsPad <span className="agent-label">Assistant</span>
        </span>
        <button
          className="icon-button"
          aria-label="Close AI assistant"
          onClick={onClose}
        >
          <X size={16} />
        </button>
      </div>
      <div className="assistant-content">
        {messages.length === 0 ? (
          <>
            <div className="assistant-task-heading">
              <h2>Explore your infrastructure</h2>
              <p>
                <FolderOpen size={15} />
                {connectionName}
              </p>
            </div>
            <button
              className="assistant-prompt-card"
              onClick={() =>
                void ask(
                  "Help me explore this connection and write a read-only query.",
                )
              }
            >
              Help me explore this connection, understand the schema, and write
              a read-only query.
            </button>
            <div className="assistant-identity">
              <span>
                <Layers3 size={17} />
              </span>
              <strong>DevOpsPad</strong>
            </div>
            <p className="ai-description">
              I can help you write queries and understand your infrastructure.
              Choose a starting point, or ask a question below.
            </p>
            <span className="eyebrow">SUGGESTED PROMPTS</span>
            {[
              "Summarize the available resources",
              "Help me write a query",
              "Explain how to explore this database",
            ].map((text, i) => (
              <button
                className="suggestion"
                key={text}
                onClick={() => void ask(text)}
              >
                {i === 0 ? (
                  <Database size={16} />
                ) : i === 1 ? (
                  <Code2 size={16} />
                ) : (
                  <Sparkles size={16} />
                )}
                <span>{text}</span>
                <ArrowUpRight size={14} />
              </button>
            ))}
            {activeTitle && (
              <div className="active-query-card">
                <div>
                  <FileCode2 size={22} />
                  <span>
                    <strong>{activeTitle}</strong>
                    <small>Current query · {engines[kind].name}</small>
                  </span>
                </div>
                <button className="button" onClick={onFocusQuery}>
                  Open current query
                  <ArrowUpRight size={14} />
                </button>
              </div>
            )}
            <p className="assistant-note">
              <ShieldCheck size={14} />
              Suggestions stay in your editor. You decide what runs.
            </p>
          </>
        ) : (
          messages.map((m, i) => (
            <div className={`ai-message ${m.role}`} key={i}>
              <span className="message-author">
                {m.role === "user" ? (
                  "You"
                ) : (
                  <>
                    <Sparkles size={13} /> DevOpsPad AI{" "}
                  </>
                )}
              </span>
              {m.content.split(/(```[\s\S]*?```)/g).map((part, j) => {
                if (part.startsWith("```")) {
                  const code = part
                    .replace(/^```\w*\n?/, "")
                    .replace(/```$/, "")
                    .trim();
                  return (
                    <div className="ai-code" key={j}>
                      <pre>{code}</pre>
                      <button onClick={() => onInsert(code)}>
                        <Code2 size={13} /> Insert into editor
                        <ArrowUpRight size={13} />
                      </button>
                    </div>
                  );
                }
                return <p key={j}>{part}</p>;
              })}
            </div>
          ))
        )}
        {busy && (
          <div className="ai-thinking">
            <LoaderCircle size={15} className="spin" /> Thinking through your
            question…
          </div>
        )}
        {error && (
          <div className="error-box" role="alert">
            {error}
            <button className="text-button" onClick={onSettings}>
              Configure provider
            </button>
          </div>
        )}
      </div>
      <div className="assistant-compose">
        <label className="context-toggle">
          <input
            type="checkbox"
            checked={context}
            onChange={(e) => setContext(e.target.checked)}
          />
          <span>{context && <Check size={11} />} Include resource names</span>
          <small>{names.length}</small>
        </label>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void ask(prompt);
          }}
        >
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Message DevOpsPad…"
            aria-label="Message AI assistant"
            maxLength={16000}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void ask(prompt);
              }
            }}
          />
          <div>
            <span>
              <span className="status-dot" />
              {engines[kind].name} context
            </span>
            <button aria-label="Send message" disabled={!prompt.trim() || busy}>
              <ArrowUp size={16} />
            </button>
          </div>
        </form>
        <div className="ai-footnote">
          <span>AI can make mistakes. Review suggestions.</span>
          <button
            className="icon-button"
            aria-label="AI provider settings"
            onClick={onSettings}
          >
            <Settings2 size={13} />
          </button>
        </div>
      </div>
    </aside>
  );
}
