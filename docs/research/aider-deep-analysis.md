# Aider — Deep Technical Analysis

> Stand: 2026-02-14

## Uebersicht

- **URL:** https://aider.chat / https://github.com/Aider-AI/aider
- **Stars:** ~40.000+ | **Lizenz:** Apache 2.0
- **Sprache:** Python (100%)
- **Installation:** `pip install aider-chat` / Docker / pipx
- **Erstellt von:** Paul Gauthier
- **Status:** Aktive Entwicklung (letzte stabile Version 0.86.1 — Entwicklungspause ab August 2025 loeste Community-Diskussionen aus)
- **Selbst-Coding:** Aider schreibt ~70% seines eigenen neuen Codes pro Release

---

## 1. Architektur

### 1.1 Tech Stack

| Komponente | Technologie |
|---|---|
| Sprache | Python 3.9+ |
| LLM-Anbindung | LiteLLM (127+ Provider) |
| Code-Parsing | tree-sitter (via py-tree-sitter-languages / tree-sitter-language-pack) |
| Graph-Ranking | NetworkX (PageRank-Algorithmus) |
| Terminal-UI | prompt_toolkit (Autocomplete, History, Farbausgabe) |
| Browser-UI | Experimentell (`--browser` Flag) |
| Git-Integration | GitPython / native git CLI |
| Spracherkennung | Whisper API (OpenAI) |
| Web-Scraping | Playwright (optional) |
| Paketierung | pip / pipx / Docker |

### 1.2 Interne Modulstruktur

```
aider/
├── main.py                    # Entry Point, CLI-Argument-Parsing
├── coders/                    # Edit-Format-Implementierungen (Kernmodul)
│   ├── base_coder.py         # Abstrakte Basis-Klasse (~hoechtse Komplexitaet)
│   ├── base_prompts.py       # System-Prompts fuer Base Coder
│   ├── editblock_coder.py    # SEARCH/REPLACE Block Format ("diff")
│   ├── wholefile_coder.py    # Ganzes File ersetzen ("whole")
│   ├── udiff_coder.py        # Unified Diff Format ("udiff")
│   ├── architect_coder.py    # Zwei-Modell Architect/Editor Pattern
│   ├── ask_coder.py          # Frage-Modus (keine Edits)
│   ├── help_coder.py         # Hilfe-Modus (Aider-Dokumentation)
│   ├── context_coder.py      # File-Auswahl mit Reflection
│   └── [format]_prompts.py   # Format-spezifische Prompts
├── models.py                  # LLM-Provider-Abstraktion via LiteLLM
├── commands.py                # In-Chat Befehle (/add, /drop, /undo, etc.)
├── io.py                      # Terminal I/O (prompt_toolkit)
├── repomap.py                 # Repository Map (tree-sitter + PageRank)
├── repo.py                    # Git-Operationen
├── linter.py                  # Lint-Integration (built-in + custom)
├── scrape.py                  # Web-Scraping (Playwright + Markdown-Konvertierung)
├── voice.py                   # Spracheingabe (Whisper)
├── resources/                 # Konfigurationsdateien, Model-Metadaten
│   ├── model-settings.yml    # Default Model-Konfigurationen
│   └── model-metadata.json   # Context-Window-Groessen, Pricing
└── website/                   # Dokumentation (aider.chat)
```

### 1.3 Coder-Klassenhierarchie

```
Coder (base_coder.py)          # Factory: Coder.create(edit_format=...)
├── EditBlockCoder             # SEARCH/REPLACE Bloecke (Standard fuer GPT-4o)
├── WholeFileCoder             # Komplettes File (Standard fuer GPT-3.5)
├── UdiffCoder                 # Unified Diff (Standard fuer GPT-4 Turbo)
├── ArchitectCoder             # Zwei-Modell-Pipeline (Architect → Editor)
├── AskCoder                   # Frage-Modus (read-only)
├── HelpCoder                  # Hilfe-Modus (Aider-Docs)
└── ContextCoder               # File-Auswahl mit Reflection
```

**Factory Pattern:** `Coder.create()` waehlt die Implementierung dynamisch basierend auf `edit_format`. Jede Subklasse definiert:
1. `edit_format` Attribut — Identifiziert die Strategie
2. `get_edits()` Methode — Extrahiert Code-Aenderungen aus LLM-Response
3. `apply_edits()` Methode — Wendet extrahierte Aenderungen auf Dateien an

### 1.4 Message-Processing-Pipeline

```
run()
  → get_input()           # User-Input lesen (Terminal/Watch/Script)
  → Command-Routing       # Wenn "/" Prefix: Commands.run()
  → run_one()             # Preprocessing
    → send_message()      # Haupt-LLM-Interaktion
      → format_messages()
        → format_chat_chunks()
          → get_repo_messages()    # Repo Map generieren
        → System Messages         # Prompts + Kontext
        → Done Messages            # Zusammengefasste History
        → Current Messages         # Aktuelle Konversation
      → LiteLLM API Call          # Streaming oder Batch
      → get_edits()               # Format-spezifisches Parsing
      → apply_edits()             # Dateien aendern
      → Auto-Lint                 # Lint-Check der geaenderten Files
      → Auto-Test                 # Test-Suite ausfuehren (optional)
      → Git Commit                # Auto-Commit mit generierten Messages
    → Reflection Loop             # Bei Fehlern: bis zu max_reflections=3 Versuche
```

### 1.5 State-Management

| Attribut | Typ | Zweck |
|---|---|---|
| `abs_fnames` | set | Absolute Pfade editierbarer Dateien |
| `abs_read_only_fnames` | set | Referenz-Dateien (nur Kontext) |
| `main_model` | Model | Primaeres LLM |
| `repo` | GitRepo | Git-Repository-Interface |
| `repo_map` | RepoMap | Codebase-Kontext-Generator |
| `commands` | Commands | Command-Handler |
| `io` | InputOutput | Terminal-Interaktion |
| `done_messages` | list | Abgeschlossene Chat-History |
| `cur_messages` | list | Aktuelle Konversation |
| `max_reflections` | int | Max. Korrektur-Versuche (Default: 3) |

---

## 2. Kernkonzepte

### 2.1 Edit Formats

Aider unterstuetzt verschiedene Strategien, wie LLMs Code-Aenderungen ausdruecken. Jedes Format ist ein Kompromiss zwischen Einfachheit, Effizienz und Modell-Kompatibilitaet.

#### 2.1.1 Whole Format (`--edit-format whole`)
- **Prinzip:** LLM gibt komplettes File zurueck, auch wenn nur wenige Zeilen geaendert wurden
- **Syntax:** Dateiname vor Code-Fence, dann kompletter Dateiinhalt
- **Default fuer:** GPT-3.5
- **Vorteile:** Simpelstes Format, geringste Fehlerquote beim Parsing
- **Nachteile:** Hoher Token-Verbrauch, langsam bei grossen Dateien, teuer

#### 2.1.2 Diff Format / Search-Replace Blocks (`--edit-format diff`)
- **Prinzip:** LLM gibt SEARCH/REPLACE-Bloecke zurueck — sucht exakten Text und ersetzt ihn
- **Syntax:**
  ```
  path/to/file.py
  <<<<<<< SEARCH
  original code
  =======
  replacement code
  >>>>>>> REPLACE
  ```
- **Default fuer:** GPT-4o, Claude Sonnet
- **Vorteile:** Effizient, nur geaenderte Teile werden uebertragen
- **Nachteile:** Erfordert exakte String-Matches, empfindlich gegen Whitespace-Fehler

#### 2.1.3 Diff-Fenced Format (`--edit-format diff-fenced`)
- **Prinzip:** Wie Diff, aber Dateiname innerhalb der Code-Fence
- **Default fuer:** Gemini-Modelle
- **Grund:** Gemini-Modelle haben Schwierigkeiten mit Standard-Diff-Fencing

#### 2.1.4 Unified Diff Format (`--edit-format udiff`)
- **Prinzip:** Basiert auf Standard Unified Diff, aber vereinfacht
- **Syntax:** `---`/`+++` Marker, `@@` Hunk-Headers, `+`/`-` Zeilenmarkierungen
- **Default fuer:** GPT-4 Turbo Familie
- **Vorteile:** Reduziert "Lazy Coding" (Modelle elidierten grosse Code-Bloecke mit `# ... original code ...` Kommentaren)
- **Nachteile:** Komplexeres Parsing, hoehere Fehlerrate bei manchen Modellen

#### 2.1.5 Udiff-Simple Format (`--edit-format udiff-simple`)
- **Variante:** Vereinfachte Version von Udiff
- **Default fuer:** Gemini 2.5 Pro

#### 2.1.6 Patch Format (`--edit-format patch`)
- **Neues Format:** Speziell fuer OpenAI GPT-4.1

#### 2.1.7 Editor-Diff und Editor-Whole (`--editor-edit-format`)
- **Prinzip:** Streamlined Versionen von Diff/Whole fuer Architect Mode
- **Prompt:** Einfacher, fokussiert nur auf File-Editing (kein Problem-Solving)
- **Verwendung:** In Kombination mit `--editor-edit-format` bei Architect Mode

#### 2.1.8 Architect Format (`--edit-format architect` / `--architect`)
- **Prinzip:** Zwei-Schritt-Prozess mit zwei LLM-Aufrufen (siehe Abschnitt 2.5)

### 2.2 Repository Map (tree-sitter + PageRank)

Die Repo Map ist Aiders innovativste Komponente — ein kompakter, token-budgetierter Ueberblick ueber die gesamte Codebasis.

#### Technische Pipeline

**Schritt 1: Code-Parsing mit tree-sitter**
- tree-sitter parst Quellcode in Abstract Syntax Trees (ASTs)
- Modifizierte `tags.scm` Dateien (aus Open-Source tree-sitter Implementierungen) identifizieren:
  - **Definitionen (`def`):** Funktionen, Klassen, Variablen, Typen
  - **Referenzen (`ref`):** Verwendungen dieser Symbole anderswo im Code
- Ergebnis: Tag-Eintraege wie `Tag(rel_fname='app/we.py', fname='/path/app/we.py', line=6, name='we', kind='def')`
- Unterstuetzte Sprachen: 100+ (Python, JS, TS, Java, C/C++, Go, Rust, etc.)

**Schritt 2: Graph-Aufbau**
- Dateien werden als Knoten im Graphen repraesentiert
- Kanten verbinden Dateien, die Abhaengigkeiten teilen (eine Datei definiert Symbol X, andere referenziert es)
- Gewichtung der Kanten:
  - Referenzierte Identifier: **10x** Gewicht
  - Lange Identifier: **10x** Gewicht (laengere Namen sind spezifischer)
  - Dateien im Chat: **50x** Gewicht (Fokus auf aktiven Kontext)
  - Private Identifier (mit Unterstrich): **1/10** Gewicht

**Schritt 3: Ranking mit PageRank**
- NetworkX PageRank-Algorithmus auf dem Datei-Graphen
- Ergebnis: Sortierte Liste der wichtigsten Code-Definitionen
- Hoeher gerankte Dateien/Symbole erscheinen zuerst in der Map

**Schritt 4: Token-Budget-Optimierung (Binary Search)**
- Konfigurierbares Token-Budget via `--map-tokens` (Default: 1024 Tokens)
- Aider passt das Budget dynamisch an:
  - Wenn keine Dateien im Chat: Map wird deutlich erweitert
  - Mit vielen Chat-Dateien: Map wird komprimiert
- **Binary Search** zwischen unterer und oberer Grenze der gerankte Tags:
  - Testen ob Token-Count innerhalb `max_map_tokens` passt
  - Bester Tree der passt wird behalten
- Caching: Geparste Symbole werden gecacht, um wiederholtes Parsing zu vermeiden

**Schritt 5: Output-Format**
- Liste von Dateien mit ihren wichtigsten Symbol-Definitionen
- Zeigt kritische Code-Zeilen fuer jede Definition (Signaturen, Klassen-Deklarationen)
- LLM kann daraus API-Nutzung, Modul-Struktur und Abhaengigkeiten ableiten

#### Map-Refresh-Modi
| Modus | Beschreibung |
|---|---|
| `auto` (Default) | Aktualisierung wenn sich Dateien aendern |
| `always` | Bei jeder Nachricht neu generieren |
| `files` | Nur wenn Chat-Dateien sich aendern |
| `manual` | Nur auf explizite Anforderung |

### 2.3 Chat-Modi

| Modus | Befehl | Beschreibung |
|---|---|---|
| **Code** (Default) | `/code` | LLM aendert Dateien direkt |
| **Architect** | `/architect` | Zwei-Modell-Pipeline: Planen + Editieren |
| **Ask** | `/ask` | Fragen beantworten, keine Aenderungen |
| **Help** | `/help` | Fragen ueber Aider selbst |

- **Einzel-Message-Override:** `/ask warum ist diese Funktion langsam?` sendet eine Nachricht im Ask-Modus, dann zurueck zum aktiven Modus
- **Persistent Switch:** `/chat-mode architect` wechselt dauerhaft
- **CLI-Launch:** `aider --chat-mode architect` oder `aider --architect`

**Empfohlener Workflow:** Ask-Modus fuer Diskussion und Planung, dann Code-Modus fuer Umsetzung. Die Konversation aus dem Ask-Modus fliesst als Kontext in den Code-Modus ein.

### 2.4 Context-Window-Management

Aider verwaltet das LLM-Context-Window aktiv:

**Automatische Zusammenfassung:**
- Wenn Chat-History die konfigurierte Token-Grenze ueberschreitet (`--max-chat-history-tokens`)
- "Weak Model" (guenstigeres Modell) erstellt Zusammenfassungen aelterer Nachrichten
- Juengste Nachrichten bleiben verbatim, aeltere werden komprimiert
- Fallback auf "Strong Model" wenn Weak Model versagt

**Manuelle Steuerung:**
- `/drop` — Dateien aus dem Chat entfernen
- `/clear` — Konversations-History loeschen
- `/tokens` — Token-Verbrauch anzeigen
- `/add` / `/read-only` — Dateien hinzufuegen (editierbar vs. read-only)

**Best Practice:** Nur relevante Dateien zum Chat hinzufuegen. Die Repo Map liefert automatisch Kontext ueber den Rest der Codebasis. Ab ~25k Tokens Context verlieren die meisten Modelle Fokus.

### 2.5 Architect/Editor Pattern

Das Architect/Editor-Pattern ist Aiders wichtigste architektonische Innovation fuer die Trennung von Reasoning und Code-Editing.

**Problem:** LLMs muessen gleichzeitig (a) das Coding-Problem loesen und (b) die Loesung in einem praezisen Edit-Format formulieren. Diese Doppelbelastung reduziert die Qualitaet beider Aufgaben.

**Loesung: Zwei-Schritt-Pipeline**

```
User Request
     |
     v
[Architect Model]  ← Stark im Reasoning (z.B. o1, Claude Opus)
     |  Beschreibt Loesung in natuerlicher Sprache
     v
[Editor Model]     ← Stark im Format-Konformitaet (z.B. DeepSeek, Sonnet)
     |  Uebersetzt in praezise SEARCH/REPLACE Bloecke
     v
File Changes
```

**Benchmark-Ergebnisse (Aider Code Editing Benchmark):**

| Kombination | Edit Format | Score |
|---|---|---|
| o1-preview + o1-mini | whole | 85.0% (SOTA) |
| o1-preview + DeepSeek | whole | 85.0% (SOTA) |
| o1-preview + Claude 3.5 Sonnet | diff | 82.7% |
| Sonnet self-paired | diff | 80.5% (vs. 77.4% solo) |
| GPT-4o self-paired | diff | 75.2% (vs. 71.4% solo) |
| GPT-4o-mini self-paired | diff | 60.2% (vs. 55.6% solo) |

**Erkenntnis:** DeepSeek ist ueberraschend effektiv als Editor — kann Loesungsbeschreibungen praezise in File-Edits uebersetzen, ohne selbst die Loesung verstehen zu muessen.

**Auto-Accept:** `--auto-accept-architect` (Default: true) — Architect-Vorschlaege werden automatisch an den Editor weitergeleitet, ohne User-Bestaetigung.

---

## 3. Git-Integration

Aider hat die tiefste Git-Integration aller AI-Coding-Tools.

### 3.1 Auto-Commits

- **Default:** Jede LLM-Aenderung wird automatisch committed
- **Commit Messages:** Vom "Weak Model" generiert, basierend auf Diffs und Chat-History
- **Format:** Conventional Commits Standard
- **Custom Prompt:** `--commit-prompt` fuer eigene Commit-Message-Templates
- **Deaktivierung:** `--no-auto-commits`

### 3.2 Dirty-File-Handling

- Vor jeder LLM-Aenderung: Aider committed zuerst existierende uncommitted Changes
- Separater Commit mit beschreibender Message
- **Deaktivierung:** `--no-dirty-commits`

### 3.3 Attribution

| Option | Beschreibung |
|---|---|
| `--attribute-author` (Default: on) | Haengt "(aider)" an Git Author-Name an |
| `--attribute-committer` | Haengt "(aider)" an Committer-Name an |
| `--attribute-commit-message-author` | Prefixed Messages mit "aider: " fuer aider-authored Changes |
| `--attribute-commit-message-committer` | Prefixed alle Messages mit "aider: " |
| `--attribute-co-authored-by` (Default: on) | Fuegt Co-authored-by Trailer hinzu |
| `--no-attribute-author` | Deaktiviert Author-Attribution |

### 3.4 Undo/Review

- `/undo` — Macht letzten LLM-Commit sofort rueckgaengig
- `/diff` — Zeigt Aenderungen seit letzter Nachricht
- `/commit` — Committed Dirty Files mit generierten Messages
- `/git <cmd>` — Fuehrt beliebige Git-Befehle aus

### 3.5 .aiderignore

- Analog zu `.gitignore` — Dateien die Aider ignorieren soll
- Default: `.aiderignore` im Git-Root
- Konfigurierbar: `--aiderignore <path>`

### 3.6 Subtree-Modus

- `--subtree-only` — Beschraenkt Aider auf das aktuelle Unterverzeichnis
- Nuetzlich fuer Monorepos oder wenn nur ein Teil bearbeitet werden soll

---

## 4. LLM-Support & Model-Konfiguration

### 4.1 Provider-Anbindung

Aider nutzt **LiteLLM** als universelle Abstraktionsschicht:
- 127+ Provider: OpenAI, Anthropic, Google, AWS Bedrock, Azure, Ollama, LM Studio, vLLM, etc.
- OpenAI-kompatible API als einheitliche Schnittstelle
- Jeder Provider der OpenAI-Format spricht, funktioniert automatisch

### 4.2 Model-Auswahl-Logik

1. Explizit: `--model <model-name>`
2. Automatisch: Aider prueft verfuegbare API-Keys (Umgebung, Config, CLI)
3. Fallback: OpenRouter-Onboarding (Free Tier: `deepseek/deepseek-r1:free`, Paid: `anthropic/claude-sonnet-4`)

### 4.3 Model-Konfiguration

**Drei Konfigurationsebenen:**

#### a) `.aider.model.settings.yml` — Verhaltens-Konfiguration
```yaml
- name: anthropic/claude-sonnet-4-20250514
  edit_format: diff
  weak_model_name: anthropic/claude-haiku-3.5
  editor_model_name: anthropic/claude-sonnet-4-20250514
  editor_edit_format: editor-diff
  use_repo_map: true
  use_temperature: true
  streaming: true
  cache_control: true
  examples_as_sys_msg: true
  lazy: false
  overeager: false
  reminder: user
  extra_params: {}
  reasoning_tag: null
  remove_reasoning: false
  accepts_settings:
    - thinking_tokens
    - reasoning_effort
```

Felder im Detail:
| Feld | Beschreibung |
|---|---|
| `name` | Model-Identifier (mit Provider-Prefix) |
| `edit_format` | Welches Edit-Format das Model nutzt |
| `weak_model_name` | Guenstiges Model fuer Commits/Summarization |
| `editor_model_name` | Editor-Model fuer Architect-Modus |
| `editor_edit_format` | Edit-Format fuer den Editor |
| `use_repo_map` | Ob Repo Map gesendet wird |
| `use_temperature` | Ob Temperature-Parameter unterstuetzt wird |
| `streaming` | Streaming-Responses aktivieren |
| `cache_control` | Prompt-Caching aktivieren (Anthropic/DeepSeek) |
| `lazy` | Deferred Processing Modus |
| `overeager` | Aggressive Response-Generierung |
| `examples_as_sys_msg` | Beispiele in System-Messages packen |
| `extra_params` | Beliebige Parameter fuer `litellm.completion()` |
| `reasoning_tag` | XML-Tag fuer Reasoning-Output |
| `remove_reasoning` | Reasoning aus Output entfernen |
| `accepts_settings` | Unterstuetzte Erweitert-Settings (thinking_tokens, reasoning_effort) |

#### b) `.aider.model.metadata.json` — Technische Metadaten
- Context-Window-Groessen
- Pricing (Input/Output Tokens)
- Basiert auf LiteLLM's `model_prices_and_context_window.json` (36.000+ Zeilen)
- Kann ueberschrieben werden fuer unbekannte Modelle

#### c) `.aider.conf.yml` — Allgemeine Aider-Konfiguration
- Alle CLI-Flags als YAML-Keys
- Lade-Reihenfolge: Home-Dir → Git-Root → CWD (spaetere ueberschreiben fruehere)

### 4.4 Benchmark-Ergebnisse (Polyglot Leaderboard, Stand 2026)

| Model | Score | Kosten | Edit Format |
|---|---|---|---|
| GPT-5 (High) | 88.0% | $29.08 | diff |
| GPT-5 (Medium) | 86.7% | $17.69 | diff |
| o3-Pro (High) | 84.9% | $146.32 | diff |
| Refact.ai Agent + Claude 3.7 Sonnet | 92.9% | n/a | agentic |
| DeepSeek Reasoner | 74.2% | $1.30 | diff |
| DeepSeek-V3.2 (Chat) | 70.2% | $0.88 | diff |

**Benchmark-Details:**
- 225 Exercism Coding-Aufgaben in C++, Go, Java, JavaScript, Python, Rust
- Zwei Versuche pro Problem (zweiter Versuch mit Test-Feedback)
- Testet sowohl Problem-Loesung als auch File-Editing-Faehigkeit

### 4.5 Prompt Caching

- **Provider:** Anthropic (Claude Sonnet, Haiku), DeepSeek
- **Aktivierung:** `--cache-prompts`
- **Cache-Struktur:** System Prompt → Read-Only Files → Repo Map → Editable Files
- **Cache-Warming:** `--cache-keepalive-pings N` — Pingt alle 5 Minuten, haelt Cache N×5 Minuten warm
- **Kostenersparnis:** Gecachte Tokens kosten ~10x weniger als ungecachte
- **Limitation:** Cache-Statistiken nur sichtbar wenn Streaming deaktiviert ist (`--no-stream`)

### 4.6 Reasoning-Support

- `--reasoning-effort VALUE` — Reasoning-Effort-Parameter (fuer o1/o3/Gemini)
- `--thinking-tokens VALUE` — Token-Budget fuer Thinking/Reasoning
- Thinking-Content wird angezeigt wenn Modelle es zurueckgeben
- Reasoning-Tags koennen via `reasoning_tag` konfiguriert und mit `remove_reasoning` entfernt werden

---

## 5. Multi-File-Editing

### 5.1 File-Management

- **Chat-Files (editierbar):** `/add <file>` — LLM kann diese Dateien aendern
- **Read-Only-Files:** `/read-only <file>` — Nur als Kontext, keine Aenderungen
- **Drop:** `/drop <file>` — Entfernt Dateien aus dem Chat
- **CLI-Start:** `aider file1.py file2.py` — Startet mit Dateien im Chat

### 5.2 Strategie

- Aider ermutigt, **nur relevante Dateien** hinzuzufuegen
- Repo Map liefert automatisch Kontext ueber den Rest der Codebasis
- Multi-File-Edits werden koordiniert — eine LLM-Antwort kann SEARCH/REPLACE Bloecke fuer mehrere Dateien enthalten
- Git-Commit umfasst alle geaenderten Dateien atomisch

### 5.3 Watch Mode (IDE-Integration)

- **Aktivierung:** `--watch-files`
- **Prinzip:** Aider ueberwacht alle Repo-Dateien auf AI-Kommentare
- **AI-Kommentar-Syntax:**
  - `# AI! beschreibung` (Python/Bash) — Triggert Code-Aenderung
  - `// AI! beschreibung` (JavaScript) — Triggert Code-Aenderung
  - `-- AI? frage` (SQL) — Triggert Frage-Modus
- **Multi-File:** AI-Kommentare koennen ueber mehrere Dateien verteilt werden
- **Workflow:** Im IDE AI-Kommentar schreiben → Aider erkennt und verarbeitet → Aenderungen werden angewendet
- **Kontext:** AI-Kommentare werden mit Repo Map und Chat-Kontext an LLM gesendet
- **Limitation:** Primaer fuer Code optimiert, Markdown-Editing problematisch

---

## 6. Konfiguration

### 6.1 Konfigurationsebenen (Prioritaet aufsteigend)

1. **Default-Werte** — Hartcodiert in Aider
2. **`~/.aider.conf.yml`** — Home-Verzeichnis (globale Defaults)
3. **`<git-root>/.aider.conf.yml`** — Projekt-spezifisch
4. **`<cwd>/.aider.conf.yml`** — Verzeichnis-spezifisch
5. **`.env`** Datei — `AIDER_*` Umgebungsvariablen
6. **Shell-Umgebungsvariablen** — `AIDER_*`
7. **CLI-Flags** — Hoechste Prioritaet

### 6.2 Beispiel `.aider.conf.yml`

```yaml
# Model
model: anthropic/claude-sonnet-4-20250514
weak-model: anthropic/claude-haiku-3.5
editor-model: anthropic/claude-sonnet-4-20250514

# Git
auto-commits: true
dirty-commits: true
attribute-co-authored-by: true

# Editing
edit-format: diff
auto-lint: true
auto-test: false
lint-cmd: "python: ruff check --fix"
test-cmd: "pytest"

# Context
map-tokens: 2048
map-refresh: auto
subtree-only: false

# UI
dark-mode: true
stream: true
pretty: true
```

### 6.3 Umgebungsvariablen

Jede CLI-Option hat ein `AIDER_*` Aequivalent:
- `AIDER_MODEL` → `--model`
- `AIDER_AUTO_COMMITS` → `--auto-commits`
- `AIDER_OPENAI_API_KEY` → `--openai-api-key`
- `AIDER_ANTHROPIC_API_KEY` → `--anthropic-api-key`
- etc.

---

## 7. API/Library-Nutzung

### 7.1 CLI-Scripting (offiziell unterstuetzt)

```bash
# Einmalige Aenderung
aider --message "fuege Error-Handling zu main.py hinzu" main.py

# Batch-Verarbeitung
for f in *.py; do
  aider --message "fuege Type Hints hinzu" "$f"
done

# Nicht-interaktiv
aider --yes --no-auto-commits --message "refaktoriere die Funktion" app.py
```

**Nuetzliche Scripting-Flags:**
| Flag | Beschreibung |
|---|---|
| `--message` / `-m` | Instruktion ausfuehren und beenden |
| `--message-file` / `-f` | Instruktion aus Datei lesen |
| `--yes` | Alle Prompts automatisch bestaetigen |
| `--no-stream` | Kein Streaming (fuer Pipes) |
| `--dry-run` | Vorschau ohne Aenderungen |
| `--commit` | Dirty Files committen und beenden |

### 7.2 Python API (inoffiziell, instabil)

```python
from aider.coders import Coder
from aider.models import Model
from aider.io import InputOutput

# Setup
io = InputOutput(yes=True, pretty=False)
model = Model("anthropic/claude-sonnet-4-20250514")
coder = Coder.create(
    main_model=model,
    fnames=["app.py", "utils.py"],
    io=io,
    auto_commits=True
)

# Ausfuehren
result = coder.run("implementiere die fehlende validate() Funktion")
result = coder.run("fuege Tests hinzu")
result = coder.run("/tokens")  # In-Chat-Befehle funktionieren auch
```

**WARNUNG:** Die Python API ist **nicht offiziell dokumentiert** und kann sich ohne Rueckwaerts-Kompatibilitaet aendern.

### 7.3 REST API

- **Existiert NICHT** — Es gibt keinen HTTP-Server-Modus
- **Feature Request:** GitHub Issue #1190 — Community wuenscht OpenAI-kompatiblen API-Server
- **Workaround:** Community-Loesung: FastAPI-Wrapper mit asyncio Subprocess

### 7.4 Browser-GUI

- **Experimentell:** `aider --browser` oeffnet Web-Interface
- **Status:** Nicht feature-complete, experimentell
- **Limitationen:** Nicht alle Terminal-Features verfuegbar, weniger stabil

---

## 8. Linting & Testing Integration

### 8.1 Auto-Lint

- **Default:** Aktiviert (`--auto-lint`)
- **Built-in Linter:** tree-sitter-basiert fuer die meisten Sprachen
- **Custom Linter:** `--lint-cmd <cmd>` (muss Non-Zero Exit bei Fehlern zurueckgeben)
- **Pro Sprache:** `--lint "python: ruff check" --lint "javascript: eslint"`
- **Feedback-Loop:** Lint-Fehler werden automatisch ans LLM zurueckgemeldet, das sie zu beheben versucht

### 8.2 Auto-Test

- **Default:** Deaktiviert (`--auto-test` zum Aktivieren)
- **Konfiguration:** `--test-cmd <cmd>` (z.B. `pytest`, `npm test`)
- **Feedback-Loop:** Test-Fehler (stdout/stderr) werden ans LLM gemeldet
- **Manuell:** `/test <cmd>` innerhalb des Chats
- **Reflection:** Bei Fehlern bis zu 3 automatische Korrektur-Versuche

### 8.3 Formatter-Integration

Formatter die Non-Zero Exit-Codes bei Aenderungen zurueckgeben (z.B. `black`, `prettier`) koennen als Linter genutzt werden, erfordern aber einen Shell-Script-Wrapper der doppelt ausfuehrt (1. Formatierung, 2. Check ob noch Fehler).

---

## 9. Multimodale Faehigkeiten

### 9.1 Image Support

- **Vision-faehige Modelle:** GPT-4o, Claude Sonnet, Gemini
- **Hinzufuegen:** `/add screenshot.png`, `/paste` (Clipboard), CLI-Argument
- **Use Cases:** Screenshots von UIs, Design-Mockups, Fehler-Screenshots, Diagramme
- **Limitation:** Bild-Dateien werden als Chat-Files hinzugefuegt, belasten Context-Window

### 9.2 Voice Support

- **Backend:** OpenAI Whisper API
- **Konfiguration:** `--voice-format wav`, `--voice-language en`, `--voice-input-device`
- **Workflow:** Sprechen → Whisper-Transkription → Aider verarbeitet als Text
- **Use Cases:** Hands-free Coding-Instruktionen, Feature-Requests verbal beschreiben

### 9.3 Web-Scraping

- **Befehl:** `/web <url>` — Scrapet Webseite, konvertiert zu Markdown, fuegt zum Chat hinzu
- **Backend:** Playwright (optional) oder einfacher HTTP-Fetch
- **Use Cases:** Aktuelle Dokumentation jenseits des Model-Training-Cutoffs
- **Preview:** `python -m aider.scrape https://example.com`

---

## 10. Staerken

### 10.1 Repository Map — Goldstandard fuer Codebase-Kontext
- **tree-sitter + PageRank** ist der ausgereifteste Ansatz fuer automatische Codebase-Kontextualisierung
- Kein anderes Tool kombiniert AST-Parsing mit Graph-Ranking und Token-Budget-Optimierung
- Funktioniert sprachuebergreifend (100+ Sprachen) ohne Konfiguration
- Dynamische Anpassung an Chat-Kontext (Dateien im Chat bekommen 50x Gewicht)

### 10.2 Edit Formats — Empirisch optimiert
- 7+ Edit-Formate, jeweils optimiert fuer spezifische Model-Familien
- Polyglot Benchmark als objektiver Vergleichsmassstab
- Kontinuierliche Evaluation neuer Modelle gegen bestehende Formate
- Neue Formate werden hinzugefuegt wenn Modelle es erfordern (z.B. `patch` fuer GPT-4.1)

### 10.3 Architect/Editor — Elegante Reasoning-Trennung
- Trennung von Problem-Solving und Code-Formatting erhoehte Benchmark-Scores signifikant
- Ermoeglicht Kombination von starken Reasoning-Modellen mit effizienten Code-Modellen
- Self-Pairing (gleiches Model als Architect+Editor) verbessert fast jedes Model

### 10.4 Git-Integration — Native und tiefgreifend
- Automatische Commits mit semantischen Messages
- Undo auf Knopfdruck (`/undo`)
- Attribution (Author, Co-authored-by)
- Dirty-File-Handling (committed ungespeicherte Aenderungen vor LLM-Edits)
- Jede Aenderung ist im Git-Verlauf nachvollziehbar

### 10.5 Feedback-Loop — Lint + Test + Reflection
- Auto-Lint → Auto-Fix → Auto-Test → Auto-Fix → Commit
- Bis zu 3 Reflection-Zyklen bei Fehlern
- Schliesst den Loop zwischen Code-Generierung und Qualitaetssicherung

### 10.6 Konfigurierbarkeit — Drei-Schicht-System
- Model-Settings, Model-Metadata und Aider-Config als separate Dateien
- Projekt-spezifische Overrides (`.aider.conf.yml` im Repo)
- Umgebungsvariablen fuer CI/CD-Integration

### 10.7 Prompt Caching — Kostenoptimierung
- Strategische Reihenfolge (System → Read-Only → Map → Editable) maximiert Cache-Hits
- Cache-Warming via Keepalive-Pings
- ~10x Kostenreduktion fuer gecachte Tokens

---

## 11. Schwaechen

### 11.1 Kein Web-GUI (Production-Grade)
- Terminal-first Design — Browser-UI nur experimentell
- Keine Multi-User-Faehigkeit
- Kein Dashboard, kein Projekt-Ueberblick
- Kein Real-time-Kollaboration

### 11.2 Single-User, Single-Session
- Kein Team-Support, kein Shared Context
- Kein Notification-System
- Knowledge-Transfer nur ueber Git-History
- Kein Multi-Projekt-Management

### 11.3 Keine REST API
- Nicht als Service deploybar
- Keine Integration in bestehende Toolchains ohne Subprocess-Hacks
- Python API inoffiziell und instabil

### 11.4 Kein Projektmanagement
- Kein Roadmap/Feature-Map
- Kein Task-Management, keine Issue-Integration
- Kein PM-Tool-Sync (Plane, OpenProject, etc.)
- Kein Spec-Driven-Development-Support

### 11.5 Kosten-Management begrenzt
- Per-Session Token-Display (`/tokens`)
- Keine Budget-Limits, kein Auto-Stop bei Kosten-Ueberschreitung
- Kein historisches Cost-Tracking (Dashboard)
- Kein Team/Projekt-basiertes Budget

### 11.6 Kein Agent-Orchestrierung
- Nur ein Agent (der User + Aider)
- Kein Multi-Agent-Pattern (Supervisor, Swarm, etc.)
- Kein DAG-basierter Workflow
- Keine Pipeline: Plan → Approve → Execute → Review → Deliver

### 11.7 Kein Sandbox/Container-Isolation
- Code wird direkt im lokalen Filesystem ausgefuehrt
- Kein Docker-in-Docker fuer sichere Agent-Execution
- Keine Command-Safety-Evaluation
- Git als einziger Rollback-Mechanismus

### 11.8 Projekt-Zukunft unsicher
- Entwicklungspause seit August 2025 (Version 0.86.1)
- Community-Diskussionen ueber Succession Plan
- Single-Maintainer-Risiko (Paul Gauthier)
- Keine formelle Governance-Struktur

### 11.9 Context-Window-Beschraenkungen
- Bei grossem Context verlieren Modelle Fokus (~25k Tokens)
- Kein GraphRAG oder semantisches Retrieval
- Repo Map ist token-budgetiert, nicht semantisch-optimiert
- Keine Experience-Pool / Caching erfolgreicher Runs

---

## 12. Relevanz fuer CodeForge

### 12.1 Was CodeForge uebernehmen sollte

#### A) Repository Map Konzept
Aiders tree-sitter + PageRank Repo Map ist der Goldstandard. CodeForge sollte:
- **tree-sitter-Parsing** in Python Workers fuer AST-basierte Code-Analyse
- **Graph-Ranking** (aber mit GraphRAG statt reinem PageRank) fuer semantisch tiefere Kontextualisierung
- **Token-Budget-Optimierung** mit Binary Search fuer Context-Window-Management
- **Dynamische Gewichtung** basierend auf Task-Kontext (aktive Dateien hoeher gewichtet)

#### B) Edit Format Architektur
CodeForge muss Edit-Formate verstehen wenn Aider als Agent-Backend genutzt wird:
- **Modell-spezifische Edit-Formate** in der Model-Konfiguration (analog zu `.aider.model.settings.yml`)
- **Benchmark-basierte Format-Auswahl** statt Vermutungen
- **Architect/Editor-Pattern** als Standard-Workflow fuer komplexe Tasks

#### C) Feedback-Loop Pattern
Die Lint → Fix → Test → Fix → Commit Pipeline ist direkt uebertragbar:
- **Quality Layer** in Python Workers: Lint-Check → LLM-Fix → Test → LLM-Fix
- **Configurable Reflection Cycles** (max_reflections als Parameter)
- **Structured Error Feedback** (Lint/Test Output als Context fuer naechsten LLM-Call)

#### D) Git-Integration Patterns
- **Auto-Commit mit Attribution** als Standard-Feature
- **Dirty-File-Handling** vor Agent-Execution
- **Undo-Mechanismus** ueber Git-History
- **Conventional Commits** als Default-Format

#### E) Prompt Caching Strategie
- **Strategische Prompt-Reihenfolge** (stabil → variabel) fuer maximale Cache-Hits
- **Cache-Warming** bei lang laufenden Tasks
- **Model-spezifische Cache-Konfiguration** (nicht jeder Provider unterstuetzt es)

### 12.2 Was CodeForge BESSER macht

#### A) Web-GUI statt Terminal
- Aider: Terminal-only (experimentelle Browser-UI)
- **CodeForge:** Full Web-GUI mit SolidJS, Real-time Updates via WebSocket, Dashboard

#### B) Multi-Projekt-Management
- Aider: Ein Repo pro Session
- **CodeForge:** Projekt-Dashboard mit mehreren Repos (Git, GitHub, GitLab, SVN, lokal)

#### C) Agent-Orchestrierung statt Single-Agent
- Aider: Ein Agent (User + Aider)
- **CodeForge:** Multi-Agent mit DAG-Orchestrierung, Plan → Approve → Execute → Review → Deliver

#### D) Aider ALS Agent-Backend
- CodeForge nutzt Aider nicht als Konkurrent sondern als **Agent-Backend**
- Aider via CLI-Scripting (`--message`) oder Python API als Worker
- Aider's Git-Integration, Repo Map und Edit-Formate als Ausfuehrungsschicht
- CodeForge liefert Orchestrierung, UI, Projektmanagement darueber

#### E) Roadmap/Spec-Driven Development
- Aider: Kein Projektmanagement, kein Spec-Support
- **CodeForge:** Bidirektionaler Sync mit PM-Tools, OpenSpec/SpecKit Support, Auto-Detection

#### F) Sandbox-Execution
- Aider: Kein Container-Isolation
- **CodeForge:** Docker-in-Docker, Command Safety Evaluator, Tool-Blocklists

#### G) Kosten-Management
- Aider: Basic Token-Display
- **CodeForge:** Budget-Limits pro Task/Projekt/User, Cost Dashboard, LiteLLM-Integration

#### H) Multi-LLM mit Scenario-Routing
- Aider: Ein Modell pro Session (optional Architect+Editor)
- **CodeForge:** Scenario-basiertes Routing (default/background/think/longContext/review/plan) via LiteLLM

### 12.3 Integrations-Strategie: Aider als Agent-Backend

```
CodeForge Go Core
       |
       v  Task Assignment via NATS/Redis
Python AI Worker
       |
       v  Subprocess / Python API
Aider (CLI oder Coder-Klasse)
       |
       ├── tree-sitter Repo Map       (Context)
       ├── Edit Format (diff/whole)    (Code-Editing)
       ├── Git Auto-Commit             (Versionierung)
       ├── Auto-Lint + Auto-Test       (Quality)
       └── LLM Call via LiteLLM        (AI)
```

**Integrationspfade:**

| Methode | Stabilitaet | Use Case |
|---|---|---|
| `aider --message "..." <files>` | Stabil, offiziell | Einfache Tasks, Batch |
| `Coder.create()` + `coder.run()` | Inoffiziell, kann sich aendern | Komplexe Workflows, Chaining |
| Subprocess mit stdin/stdout | Stabil, aber fragil | Server-Integration |

**Empfehlung:** CLI-Scripting (`--message`) fuer robuste Integration, Python API nur wenn noetig und mit Versions-Pinning.

### 12.4 Architektur-Erkenntnisse fuer CodeForge

| Aider-Konzept | CodeForge-Adaption |
|---|---|
| Repo Map (tree-sitter + PageRank) | GraphRAG Context Layer (tiefer, semantisch) |
| Edit Formats (7+ Varianten) | Modell-spezifische Format-Config in Worker-Settings |
| Architect/Editor Pattern | Standard-Workflow im Agent-Pipeline (Plan → Edit) |
| Auto-Lint + Auto-Test Loop | Quality Layer mit konfigurierbaren Reflection Cycles |
| Prompt Caching (Anthropic/DeepSeek) | Cache-Strategie via LiteLLM Proxy delegieren |
| `.aider.conf.yml` + `.env` | YAML-basierte Worker-Config + Environment Variables |
| Watch Mode (AI-Kommentare) | Nicht relevant (CodeForge hat eigene UI) |
| Voice/Image | Spaetere Phase, nicht Kern-Feature |
| Weak Model (Commits/Summary) | Scenario-Routing: background Tag fuer guenstige Ops |

---

## 13. Zusammenfassung

### Aider in einem Satz
Terminal-basierter AI Pair-Programmer mit der tiefsten Git-Integration und dem ausgereiftesten Codebase-Kontext-System (tree-sitter + PageRank Repo Map) aller Open-Source-Tools, aber ohne Web-GUI, Projektmanagement oder Agent-Orchestrierung.

### Zahlen
- 40.000+ GitHub Stars
- 100+ unterstuetzte Sprachen (tree-sitter)
- 127+ LLM-Provider (via LiteLLM)
- 7+ Edit-Formate (modellspezifisch optimiert)
- 225 Polyglot Benchmark-Aufgaben (6 Sprachen)
- ~70% Self-Coded (pro Release)
- Apache 2.0 Lizenz

### Kernkonzepte fuer CodeForge
1. **Repo Map (tree-sitter + PageRank)** — Goldstandard fuer Code-Kontext
2. **Edit Format Architektur** — Modellspezifisch, benchmark-basiert
3. **Architect/Editor Pattern** — Reasoning/Editing Trennung
4. **Lint/Test Feedback Loop** — Auto-Fix mit Reflection Cycles
5. **Git-native Workflow** — Auto-Commit, Attribution, Undo
6. **Aider als Agent-Backend** — Via CLI oder Python API in Worker integrierbar
