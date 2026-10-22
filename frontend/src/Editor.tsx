import { useEffect, useRef } from "react";
import { EditorView, keymap } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { basicSetup } from "codemirror";
import { sql, MySQL } from "@codemirror/lang-sql";
import { json } from "@codemirror/lang-json";
import type { Kind } from "./types";

const orcaHighlight = HighlightStyle.define([
  { tag: tags.keyword, color: "#c4b5fd" },
  { tag: tags.string, color: "#86efac" },
  { tag: tags.number, color: "#fdba74" },
  { tag: tags.comment, color: "#737373", fontStyle: "italic" },
  { tag: tags.operator, color: "#a1a1a1" },
  { tag: [tags.propertyName, tags.attributeName], color: "#93c5fd" },
  { tag: [tags.variableName, tags.name], color: "#fafafa" },
  { tag: [tags.bool, tags.null], color: "#fdba74" },
]);

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
          syntaxHighlighting(orcaHighlight),
          EditorView.theme({
            "&": {
              height: "100%",
              background: "#0a0a0a",
              color: "#fafafa",
              fontSize: "13px",
            },
            ".cm-scroller": {
              fontFamily:
                'ui-monospace, "SF Mono", SFMono-Regular, Menlo, Consolas, monospace',
              lineHeight: "1.7",
              overflow: "auto",
            },
            ".cm-content": { padding: "16px 0", caretColor: "#fafafa" },
            ".cm-line": { paddingLeft: "20px" },
            ".cm-gutters": {
              background: "#0a0a0a",
              border: "none",
              color: "#737373",
              minWidth: "50px",
            },
            ".cm-activeLineGutter": { background: "#171717" },
            ".cm-activeLine": { background: "#ffffff08" },
            "&.cm-focused": { outline: "none" },
            ".cm-cursor": { borderLeftColor: "#fafafa" },
            ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
              background: "#ffffff18",
            },
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
