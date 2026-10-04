import { useEffect, useRef } from "react";
import { EditorView, keymap } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { basicSetup } from "codemirror";
import { sql, MySQL } from "@codemirror/lang-sql";
import { json } from "@codemirror/lang-json";
import type { Kind } from "./types";

export function Editor({
  value,
  kind,
  onChange,
  onRun,
}: {
  value: string;
  kind: Kind;
  onChange: (value: string) => void;
  onRun: () => void;
}) {
  const root = useRef<HTMLDivElement>(null);
  const editor = useRef<EditorView | null>(null);
  const change = useRef(onChange);
  change.current = onChange;
  const run = useRef(onRun);
  run.current = onRun;
  useEffect(() => {
    if (!root.current) return;
    const view = new EditorView({
      parent: root.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          kind === "mysql" || kind === "clickhouse"
            ? sql({ dialect: MySQL })
            : kind === "mongo"
              ? json()
              : [],
          keymap.of([
            {
              key: "Mod-Enter",
              run: () => {
                run.current();
                return true;
              },
            },
          ]),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) change.current(update.state.doc.toString());
          }),
          EditorView.theme({
            "&": { height: "100%", background: "#fff", fontSize: "13px" },
            ".cm-scroller": {
              fontFamily:
                '"SFMono-Regular", Consolas, "Liberation Mono", monospace',
              lineHeight: "1.8",
              overflow: "auto",
            },
            ".cm-content": { padding: "18px 0" },
            ".cm-line": { paddingLeft: "20px" },
            ".cm-gutters": {
              background: "#fff",
              border: "none",
              color: "#9399a2",
              minWidth: "50px",
            },
            ".cm-activeLineGutter": { background: "#f1f3f6" },
            ".cm-activeLine": { background: "#f8f9fb" },
            "&.cm-focused": { outline: "none" },
            ".cm-cursor": { borderLeftColor: "#454b54" },
          }),
          EditorView.contentAttributes.of({
            "aria-label": "Query editor",
            spellcheck: "false",
          }),
        ],
      }),
    });
    editor.current = view;
    return () => {
      view.destroy();
      editor.current = null;
    };
  }, [kind]);
  useEffect(() => {
    const view = editor.current;
    if (view && view.state.doc.toString() !== value)
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: value },
      });
  }, [value]);
  return <div className="query-editor" ref={root} />;
}
