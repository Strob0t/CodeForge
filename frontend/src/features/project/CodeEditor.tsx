import type { Monaco } from "@monaco-editor/loader";
import type { editor } from "monaco-editor";
import { createEffect, createSignal, type JSX, on } from "solid-js";
import { MonacoEditor } from "solid-monaco";

import { useTheme } from "~/components/ThemeProvider";

export interface CodeEditorProps {
  value: string;
  language: string;
  path: string;
  onChange: (value: string) => void;
  onSave: () => void;
}

// Map language IDs to Monaco language identifiers
function monacoLanguage(lang: string): string {
  const mapping: Record<string, string> = {
    go: "go",
    python: "python",
    javascript: "javascript",
    typescript: "typescript",
    html: "html",
    css: "css",
    scss: "scss",
    json: "json",
    yaml: "yaml",
    xml: "xml",
    markdown: "markdown",
    sql: "sql",
    shell: "shell",
    rust: "rust",
    java: "java",
    c: "c",
    cpp: "cpp",
    ruby: "ruby",
    php: "php",
    swift: "swift",
    kotlin: "kotlin",
    toml: "toml",
    dockerfile: "dockerfile",
    makefile: "makefile",
    graphql: "graphql",
    protobuf: "protobuf",
    plaintext: "plaintext",
  };
  return mapping[lang] ?? "plaintext";
}

export default function CodeEditor(props: CodeEditorProps): JSX.Element {
  const { resolved } = useTheme();
  const [editorRef, setEditorRef] = createSignal<editor.IStandaloneCodeEditor | null>(null);
  const [monacoRef, setMonacoRef] = createSignal<Monaco | null>(null);

  // Explicitly sync Monaco theme whenever the app theme changes
  createEffect(() => {
    const m = monacoRef();
    if (!m) return;
    const theme = resolved() === "dark" ? "vs-dark" : "vs";
    m.editor.setTheme(theme);
  });

  // Register Ctrl+S keybinding when editor mounts
  createEffect(
    on(editorRef, (editor) => {
      if (!editor) return;
      // Monaco KeyMod.CtrlCmd = 2048, KeyCode.KeyS = 49
      const saveFn = props.onSave;
      editor.addCommand(2048 | 49, () => {
        saveFn();
      });
    }),
  );

  return (
    <div class="h-full w-full">
      <MonacoEditor
        language={monacoLanguage(props.language)}
        value={props.value}
        theme={resolved() === "dark" ? "vs-dark" : "vs"}
        onChange={(val) => props.onChange(val ?? "")}
        onMount={(monaco, ed) => {
          setMonacoRef(monaco);
          setEditorRef(ed);
        }}
        options={{
          minimap: { enabled: false },
          fontSize: 13,
          lineNumbers: "on",
          scrollBeyondLastLine: false,
          wordWrap: "on",
          tabSize: 2,
          renderWhitespace: "selection",
          automaticLayout: true,
        }}
      />
    </div>
  );
}
