import { useEffect, useRef } from "react";
import { Compartment, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import {
  HighlightStyle,
  StreamLanguage,
  syntaxHighlighting,
} from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { basicSetup } from "codemirror";
import { yaml } from "@codemirror/lang-yaml";
import { json } from "@codemirror/lang-json";
import { toml } from "@codemirror/legacy-modes/mode/toml";
import { nginx } from "@codemirror/legacy-modes/mode/nginx";
import { properties } from "@codemirror/legacy-modes/mode/properties";
import { parseAllDocuments } from "yaml";
import { parse as parseToml } from "smol-toml";

export function configLanguage(filename: string) {
  const name = filename.toLowerCase();
  if (/\.ya?ml$/.test(name)) return "YAML";
  if (name.endsWith(".json")) return "JSON";
  if (name.endsWith(".toml")) return "TOML";
  if (name === "nginx.conf" || name.endsWith(".nginx")) return "Nginx";
  if (/\.(properties|ini|env)$/.test(name)) return "Properties";
  return "Text";
}

export function validateConfig(filename: string, value: string) {
  if (!value.trim()) return;
  switch (configLanguage(filename)) {
    case "YAML":
      for (const doc of parseAllDocuments(value)) {
        if (doc.errors.length) throw new Error(doc.errors[0].message);
      }
      break;
    case "JSON":
      JSON.parse(value);
      break;
    case "TOML":
      parseToml(value);
      break;
  }
}

const highlight = HighlightStyle.define([
  { tag: tags.keyword, color: "var(--config-keyword)" },
  { tag: tags.string, color: "var(--config-string)" },
  { tag: [tags.number, tags.bool, tags.null], color: "var(--config-number)" },
  { tag: tags.comment, color: "var(--ops-muted)", fontStyle: "italic" },
  {
    tag: [tags.propertyName, tags.attributeName, tags.heading],
    color: "var(--config-property)",
  },
]);

export function ConfigCodeEditor({
  filename,
  label,
  value,
  editable,
  onChange,
}: {
  filename: string;
  label: string;
  value: string;
  editable: boolean;
  onChange: (value: string) => void;
}) {
  const root = useRef<HTMLDivElement>(null);
  const editor = useRef<EditorView | null>(null);
  const mode = useRef(new Compartment());
  const change = useRef(onChange);
  change.current = onChange;
  useEffect(() => {
    if (!root.current) return;
    const language = configLanguage(filename);
    const view = new EditorView({
      parent: root.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          language === "YAML"
            ? yaml()
            : language === "JSON"
              ? json()
              : language === "TOML"
                ? StreamLanguage.define(toml)
                : language === "Nginx"
                  ? StreamLanguage.define(nginx)
                  : language === "Properties"
                    ? StreamLanguage.define(properties)
                    : [],
          syntaxHighlighting(highlight),
          EditorView.lineWrapping,
          mode.current.of([
            EditorState.readOnly.of(!editable),
            EditorView.editable.of(editable),
          ]),
          EditorView.contentAttributes.of({
            "aria-label": label,
            role: "textbox",
            "aria-multiline": "true",
            spellcheck: "false",
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) change.current(update.state.doc.toString());
          }),
        ],
      }),
    });
    editor.current = view;
    return () => {
      view.destroy();
      editor.current = null;
    };
  }, [filename, label]);
  useEffect(() => {
    editor.current?.dispatch({
      effects: mode.current.reconfigure([
        EditorState.readOnly.of(!editable),
        EditorView.editable.of(editable),
        EditorView.contentAttributes.of({ "aria-readonly": String(!editable) }),
      ]),
    });
  }, [editable]);
  useEffect(() => {
    const view = editor.current;
    if (view && view.state.doc.toString() !== value) {
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: value },
      });
    }
  }, [value]);
  return <div className="ops-config-editor" ref={root} />;
}
