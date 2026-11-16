# JSON response

Return exactly one JSON object, for example:

```json
{"branch":0,"text":"..."}
```

Include both fields:

- Set `branch` to the chosen zero-based integer index.
- Set `text` to your response string (empty is allowed).

For a `Stop` branch, `text` is returned as the final answer; otherwise the
continuation receives the original task, not `text`.

Output raw JSON, without a Markdown fence. Do not include markdown, commentary,
or any other output.
