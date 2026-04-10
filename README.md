# `injectctl`

`injectctl` is a local-first CLI for turning screenshots, notes, and scanner outputs into draft assessment reports or exercise inject packs. It is designed for offline-capable corporate environments and uses `Ollama` with `Gemma 4` as the default AI runtime.

## Implemented Surface

- `injectctl assess run --manifest job.yaml --input ./artifacts --out ./out`
- `injectctl inject run --manifest job.yaml --input ./artifacts --out ./out`
- `injectctl template validate --template ./template.md.tmpl`
- `injectctl doctor`
- `injectctl init manifest --mode assess|inject`

## Manifest Highlights

- `mode`: `assess` or `inject`
- `artifacts`: file paths or directories to ingest
- `ai.endpoint`: defaults to `http://127.0.0.1:11434`
- `ai.model`: defaults to `gemma4:26b`
- `ai.fallback_model`: defaults to `gemma4:e4b`
- `output.formats`: `markdown`, `json`, `pdf`

See [examples/assessment-manifest.yaml](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/examples/assessment-manifest.yaml) and [examples/inject-manifest.yaml](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/examples/inject-manifest.yaml).

## Runtime Requirements

- `Ollama` running locally
- `gemma4:26b` pulled into Ollama
- Optional `gemma4:e4b` fallback
- Optional `tesseract` for screenshot OCR

## Notes

- OCR always runs first when `tesseract` is available.
- Image artifacts are also sent to Ollama for multimodal synthesis when possible.
- If model output is malformed, the tool retries once and then falls back to heuristic drafts backed by collected evidence.

## Release Builds

Run the release script from the repo root to build zipped binaries and checksums for all supported targets:

```powershell
.\scripts\release.ps1 -Version v0.1.0
```

Artifacts are written under `dist/<version>/` and include:

- `windows-amd64`
- `linux-amd64`
- `linux-arm64`
- `darwin-amd64`
- `darwin-arm64`
- `sha256sums.txt`
