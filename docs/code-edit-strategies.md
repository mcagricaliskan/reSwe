# How AI Coding Agents Apply Code Edits

Research for building reSwe's file-write tool.

## The Dominant Pattern: Search/Replace (String Match)

Almost every successful agent avoids line numbers. LLMs are bad at counting lines. Instead, the standard approach is:

```
old_str → new_str
```

Find the exact substring in the file, replace it. Simple.

---

## Agent Comparison

### 1. Claude Code (Anthropic)

**Two tools:**

| Tool | Parameters | What it does |
|------|-----------|--------------|
| `Edit` | `file_path`, `old_string`, `new_string`, `replace_all` | Exact string replacement. Fails if `old_string` isn't unique (unless `replace_all=true`) |
| `Write` | `file_path`, `content` | Full file overwrite. For new files or complete rewrites |

- No line numbers, no regex, no diff parsing
- `old_string` must be an exact match (including whitespace/indentation)
- If 0 or >1 matches, returns error — LLM must retry with more context

### 2. Aider

**6 pluggable edit formats**, picks per model:

| Format | How it works | Best for |
|--------|-------------|----------|
| `diff` (default) | SEARCH/REPLACE blocks with `<<<<<<< SEARCH` / `>>>>>>> REPLACE` markers | Most models |
| `udiff` | Unified diff format (`-`/`+` lines) | GPT-4 Turbo |
| `whole` | LLM returns entire file content | Small files, weak models |
| `diff-fenced` | Same as diff but filepath inside fence | Gemini |
| `editor-diff` | Architect LLM plans, editor LLM applies | Two-pass mode |
| `editor-whole` | Same but editor outputs whole file | Two-pass mode |

Key insight: Different models work better with different formats.

### 3. SWE-Agent

**`str_replace_editor` tool** (Anthropic adopted this exact interface):

| Command | Parameters |
|---------|-----------|
| `view` | `path`, `view_range` |
| `create` | `path`, `file_text` |
| `str_replace` | `path`, `old_str`, `new_str` |
| `insert` | `path`, `insert_line`, `insert_text` |
| `undo_edit` | `path` |

Innovation: `undo_edit` lets agent recover from mistakes.

### 4. OpenHands

**`FileEditorTool`** with 6 operations:

| Operation | Parameters | Method |
|-----------|-----------|--------|
| `read` | `path`, `start_line`, `end_line` | Line-range reading |
| `write` | `path`, `content` | Full file write |
| `insert` | `path`, `line_number`, `content` | Insert at line |
| `replace` | `path`, `old_content`, `new_content` | String match |
| `delete` | `path`, `start_line`, `end_line` | Remove line range |
| `search` | `path`, `pattern` | Search for text |

### 5. Cursor

**Two-stage architecture** (unique approach):

1. **Sketch**: Frontier model (Claude/GPT-4) outputs search/replace blocks with `-`/`+` lines. No line numbers.
2. **Apply**: Custom finetuned Llama-3-70b takes sketch + original file → outputs complete updated file. Uses speculative decoding (~1000 tok/s) by copying unchanged tokens from original.

Not replicable without training a custom model.

### 6. Cline

**Two tools:**

| Tool | What it does |
|------|-------------|
| `write_to_file` | Full file overwrite |
| `replace_in_file` | SEARCH/REPLACE blocks with fuzzy matching |

Innovation: **Fuzzy matching** — if exact match fails, progressively relaxes to find best match. Also handles out-of-order blocks.

---

## Summary Table

| Agent | Edit Method | Location Finding | Fuzzy? | Line Numbers? | Full Rewrite? |
|-------|-----------|-----------------|--------|--------------|--------------|
| Claude Code | `str_replace(old, new)` | Exact string | No | No | Yes (Write) |
| Aider | SEARCH/REPLACE blocks | Exact string | No | Only udiff | Yes (whole) |
| SWE-Agent | `str_replace(old, new)` | Exact string | No | insert only | Yes (create) |
| OpenHands | `replace(old, new)` | Exact + line nums | No | Some ops | Yes (write) |
| Cursor | Sketch → Apply model | Neural (trained) | N/A | No | Yes |
| Cline | SEARCH/REPLACE blocks | Fuzzy string | Yes | No | Yes |

---

## Key Takeaways for reSwe

### What we should build

**Two tools** (the Claude Code / Cline pattern — proven simplest and most effective):

#### `write_file`
- **Parameters**: `path` (repo-prefixed), `content` (full file)
- **Use case**: New files, or small files where full rewrite is simpler
- **Implementation**: Write content to resolved path

#### `edit_file`
- **Parameters**: `path`, `old_content`, `new_content`
- **Use case**: Surgical edits to existing files
- **Implementation**: Find `old_content` in file via exact string match, replace with `new_content`
- **Error cases**:
  - 0 matches → error: "old_content not found in file"
  - >1 matches → error: "old_content matches N locations, provide more context to make it unique"

### Why this approach

1. **No line numbers** — LLMs can't count lines reliably
2. **No diff parsing** — complex to implement, error-prone with local models
3. **Exact match first** — simple, deterministic, debuggable
4. **Full rewrite fallback** — when edits are too complex, just rewrite the file
5. **Battle-tested** — Claude Code, SWE-Agent, Aider all converge on this pattern

### What to avoid

- Unified diff format — local models (Ollama) will produce malformed diffs constantly
- Line-number-based insert/delete — LLMs miscalculate line numbers
- Regex-based matching — too much rope for the LLM to hang itself with
- Fuzzy matching (for now) — adds complexity, can match wrong locations. Add later if exact match proves too brittle.

### The prompt matters

The system prompt for execute phase must:
1. Tell the agent to `read_file` BEFORE editing (so it has exact content to match)
2. Show examples of `edit_file` usage with exact string matches
3. Emphasize: include enough context in `old_content` to be unique (3-5 lines minimum)
4. For new files: use `write_file` with complete content
