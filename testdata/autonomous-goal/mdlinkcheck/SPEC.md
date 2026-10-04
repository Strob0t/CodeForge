# mdlinkcheck - specification (the goal given to the agent)

Build `mdlinkcheck`, a command-line tool that finds broken links in Markdown files. This text is the complete goal: the
agent gets it as the project goal and nothing else.

## Deliverable

- A Python 3.12 package `mdlinkcheck` in a `src/` layout with a `pyproject.toml`, installable with `pip install -e .`.
- A console script `mdlinkcheck` and `python -m mdlinkcheck`, both with the same behaviour.
- No runtime dependencies outside the Python standard library.
- Tests with pytest, type hints on all functions, a README that explains installation and usage.

## Command line

```
mdlinkcheck [PATH ...] [--format {text,json}] [--exclude GLOB]... [--check-external] [--timeout SECONDS] [--config FILE]
```

- `PATH`: Markdown files or directories; the default is `.`. Directories are scanned recursively for files ending in
  `.md` or `.markdown`. Directories whose name starts with `.` and directories named `node_modules` are skipped.
- `--exclude GLOB`: may be repeated. A file is skipped when its path relative to the `PATH` argument it was found under
  (POSIX separators) matches the glob (`fnmatch` rules, so `*` also matches `/`).
- `--format`: `text` (default) or `json`, see Output.
- `--check-external`: also check `http` and `https` links (off by default).
- `--timeout SECONDS`: timeout per external request, default `5`.
- `--config FILE`: a TOML configuration file. Without it, `.mdlinkcheck.toml` in the current directory is used if it
  exists. Keys: `exclude` (list of globs, added to the `--exclude` options), `check_external` (bool), `timeout` (float).
  Command-line options override the file.

## What counts as a link

- Inline links `[text](target)` and images `![alt](target)`, with an optional title: `[text](target "title")` or
  `[text](target 'title')`. A target in angle brackets may contain spaces: `[text](<my file.md>)`.
- Reference definitions `[id]: target` (optionally with a title) at the start of a line, and their uses
  `[text][id]`, `[id][]` and `[id]`. Reference ids match case-insensitively and with runs of whitespace collapsed.
  A `[x]` with no definition named `x` is plain text, not a link; `[text][x]` and `[x][]` without a definition are
  problems (`undefined-reference`).
- Autolinks `<https://example.com>`.
- Not links: anything inside fenced code blocks (lines between ```` ``` ```` or `~~~` fences) or inside inline code
  spans (`` `...` ``).

## Classification of a target

- `http://` or `https://`: external.
- `mailto:` and any other scheme (`ftp:`, `tel:`, ...): ignored.
- `#fragment` alone: an anchor in the same file.
- Anything else: a local path, optionally followed by `?query` (ignored) and `#fragment`. Percent-encoding in the
  path is decoded (`my%20file.md` means `my file.md`).

## Checks and problem kinds

| Kind | When |
|---|---|
| `missing-file` | A local path does not exist. Relative paths are resolved against the directory of the file that contains the link; a path that starts with `/` is resolved against the `PATH` argument the file was found under (for a file given directly, its own directory). A path to an existing directory is fine. |
| `missing-anchor` | The fragment does not name an anchor in the target Markdown file (the same file for `#fragment`). Anchors are: the heading slugs (see below), and `name` or `id` attribute values of HTML elements in the file (for example `<a name="x"></a>`, `<span id="y">`). Fragments that point into non-Markdown files are not checked. |
| `undefined-reference` | A full or collapsed reference (`[text][id]`, `[id][]`) uses an id without a definition. |
| `external-error` | Only with `--check-external`: the request fails (connection error, timeout) or the status is 400 or higher. Send `HEAD` first; if the server answers `405`, retry with `GET`. Follow redirects. |

Heading slugs follow GitHub's rules: take the heading text (ATX headings `#` to `######`, and Setext headings
underlined with `===` or `---`), strip leading and trailing whitespace and inline Markdown markers for emphasis
and code (`*`, `_`, `` ` ``), lowercase it, remove every character that is not a letter, a digit, a space, a
hyphen or an underscore, then replace each space with a hyphen. When the same slug occurs again in a file, the
second gets `-1` appended, the third `-2`, and so on. Fragment comparison is exact (after percent-decoding).

## Details

- A reference definition is a link at its own position. Its target is checked once, where it is defined, whether or
  not it is used, and its problems are reported at its `[`. Uses are not checked again; a full or collapsed use without
  a definition is an `undefined-reference`. Each definition counts once in `links_checked`; uses do not count.
  Definitions apply only within their own file.
- Fenced code blocks contain no links, no headings and no HTML anchors. A fence closes only with the same character
  and at least as many of them; an unclosed fence runs to the end of the file.
- A letter in a heading slug is any Unicode letter (for example `ü`). `_` is removed only where it marks emphasis, so
  `snake_case` keeps it. The heading text excludes an ATX heading's closing `#`s; a link in a heading contributes its
  text only.
- Bare URLs without angle brackets are not links. An empty fragment (`#`, `a.md#`) is not checked.
- `--exclude` applies only to files found by scanning a directory; a file named directly is always checked, whatever
  its extension. A file reached through several `PATH` arguments is checked and counted once.
- Files are read as UTF-8 with invalid bytes replaced. Columns count Unicode code points.
- A configuration value of the wrong type, a non-positive timeout or an unknown key makes the configuration invalid
  (exit `2`), and so does an invalid option value such as `--format xml`. A file that cannot be read exits with `2`.

## Output

Positions are 1-based. The column is that of the first character of the link: the `!` of an image, the `[` of a
link or reference use, the `<` of an autolink. For a problem found at a definition, it is the `[` of the definition.
Paths are relative to the current working directory, with POSIX separators. Problems are sorted by path, then line,
then column.

**Text** (default): one line per problem, then a summary line.

```
docs/guide.md:12:5: missing-file: ../setup.md (file not found)
docs/guide.md:20:1: missing-anchor: api.md#usage (no anchor "usage" in api.md)
2 problems in 7 files
```

With no problems the only line is `No problems found in N files`. With one problem the summary says `1 problem`;
files are counted the same way (`1 file`, `N files`).

**JSON** (`--format json`): a single object on stdout.

```json
{
  "files_checked": 7,
  "links_checked": 31,
  "problems": [
    {"path": "docs/guide.md", "line": 12, "column": 5, "kind": "missing-file", "target": "../setup.md", "message": "file not found"}
  ]
}
```

`target` is the link target as written in the source, without angle brackets and title; percent-encoding, query and
fragment stay as written (for references: the target of the definition, or the id when the definition is missing). `links_checked` counts every link that was classified as external (only when checked),
anchor or local path.

## Exit codes

- `0`: no problems.
- `1`: at least one problem.
- `2`: usage error: an unknown option, a `PATH` that does not exist, or a configuration file that cannot be read or
  parsed. The message goes to stderr.

## Quality expectations

The result is judged on behaviour (a separate acceptance test suite against this specification), on its own tests and
their coverage, on `ruff check`, `mypy --strict`, function complexity, a security scan and a code review. Keep the code
readable, small functions, no dead code, clear error messages.
